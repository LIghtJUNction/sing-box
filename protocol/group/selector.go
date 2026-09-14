package group

import (
	"context"
	"net"
	"regexp"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/interrupt"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

func RegisterSelector(registry *outbound.Registry) {
	outbound.Register[option.SelectorOutboundOptions](registry, C.TypeSelector, NewSelector)
}

var (
	_ adapter.OutboundGroup           = (*Selector)(nil)
	_ adapter.Referrer                = (*Selector)(nil)
	_ adapter.ConnectionHandler       = (*Selector)(nil)
	_ adapter.PacketConnectionHandler = (*Selector)(nil)
)

type Selector struct {
	outbound.Adapter
	ctx                          context.Context
	outbound                     adapter.OutboundManager
	connection                   adapter.ConnectionManager
	logger                       logger.ContextLogger
	tags                         []string
	defaultTag                   string
	outbounds                    map[string]adapter.Outbound
	selected                     common.TypedValue[adapter.Outbound]
	history                      *urltest.HistoryStorage
	interruptGroup               *interrupt.Group
	interruptExternalConnections bool

	provider       adapter.ProviderManager
	providers      map[string]adapter.Provider
	outboundsCache map[string][]adapter.Outbound

	providerTags    []string
	exclude         *regexp.Regexp
	include         *regexp.Regexp
	useAllProviders bool
}

func NewSelector(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.SelectorOutboundOptions) (adapter.Outbound, error) {
	outbound := &Selector{
		Adapter:                      outbound.NewAdapter(C.TypeSelector, tag, []string{N.NetworkTCP, N.NetworkUDP}, options.Outbounds),
		ctx:                          ctx,
		outbound:                     service.FromContext[adapter.OutboundManager](ctx),
		connection:                   service.FromContext[adapter.ConnectionManager](ctx),
		logger:                       logger,
		tags:                         options.Outbounds,
		defaultTag:                   options.Default,
		outbounds:                    make(map[string]adapter.Outbound),
		history:                      service.PtrFromContext[urltest.HistoryStorage](ctx),
		interruptGroup:               interrupt.NewGroup(),
		interruptExternalConnections: options.InterruptExistConnections,

		provider:       service.FromContext[adapter.ProviderManager](ctx),
		providers:      make(map[string]adapter.Provider),
		outboundsCache: make(map[string][]adapter.Outbound),

		providerTags:    options.Providers,
		exclude:         (*regexp.Regexp)(options.Exclude),
		include:         (*regexp.Regexp)(options.Include),
		useAllProviders: options.UseAllProviders,
	}
	return outbound, nil
}

func (s *Selector) Network() []string {
	selected := s.selected.Load()
	if selected == nil {
		return []string{N.NetworkTCP, N.NetworkUDP}
	}
	return selected.Network()
}

func (s *Selector) Start() error {
	if s.useAllProviders {
		var providerTags []string
		for _, provider := range s.provider.Providers() {
			providerTags = append(providerTags, provider.Tag())
			s.providers[provider.Tag()] = provider
			provider.RegisterCallback(s.onProviderUpdated)
		}
		s.providerTags = providerTags
	} else {
		for i, tag := range s.providerTags {
			provider, loaded := s.provider.Get(tag)
			if !loaded {
				return E.New("outbound provider ", i, " not found: ", tag)
			}
			s.providers[tag] = provider
			provider.RegisterCallback(s.onProviderUpdated)
		}
	}
	if len(s.tags)+len(s.providerTags) == 0 {
		return E.New("missing outbound and provider tags")
	}

	for i, tag := range s.tags {
		detour, loaded := s.outbound.Outbound(tag)
		if !loaded {
			return E.New("outbound ", i, " not found: ", tag)
		}
		s.outbounds[tag] = detour
	}
	if len(s.tags) == 0 {
		detour, _ := s.outbound.Outbound("Compatible")
		s.tags = append(s.tags, detour.Tag())
		s.outbounds[detour.Tag()] = detour
	}
	outbound, err := s.outboundSelect()
	if err != nil {
		return err
	}
	s.selected.Store(outbound)
	return nil
}

func (s *Selector) Now() string {
	selected := s.selected.Load()
	if selected == nil {
		return s.tags[0]
	}
	return selected.Tag()
}

func (s *Selector) All() []string {
	return s.tags
}

func (s *Selector) References() []string {
	return []string{s.Now()}
}

func (s *Selector) SelectOutbound(tag string) bool {
	detour, loaded := s.outbounds[tag]
	if !loaded {
		return false
	}
	if s.selected.Load() == detour {
		return true
	}
	s.selected.Store(detour)
	invalidateReachability(s.ctx) // lx: SPEC 020 — active selection changed
	if s.Tag() != "" {
		cacheFile := service.FromContext[adapter.CacheFile](s.ctx)
		if cacheFile != nil {
			err := cacheFile.StoreSelected(s.Tag(), tag)
			if err != nil {
				s.logger.Error("store selected: ", err)
			}
		}
	}
	s.interruptGroup.Interrupt(s.interruptExternalConnections)
	if s.history != nil {
		s.history.NotifyUpdated()
	}
	return true
}

func (s *Selector) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	// lx:begin chain
	// SPEC 073: внутри цепочки выбранный узел подменяется его звеном для хопа.
	selected, err := adapter.ResolveChainLeaf(ctx, s.selected.Load())
	if err != nil {
		return nil, err
	}
	conn, err := selected.DialContext(ctx, network, destination)
	// lx:end chain
	if err != nil {
		return nil, err
	}
	return s.interruptGroup.NewConn(conn, interrupt.IsExternalConnectionFromContext(ctx), interrupt.IsProviderConnectionFromContext(ctx)), nil
}

func (s *Selector) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	// lx:begin chain
	selected, err := adapter.ResolveChainLeaf(ctx, s.selected.Load())
	if err != nil {
		return nil, err
	}
	conn, err := selected.ListenPacket(ctx, destination)
	// lx:end chain
	if err != nil {
		return nil, err
	}
	return s.interruptGroup.NewPacketConn(conn, interrupt.IsExternalConnectionFromContext(ctx), interrupt.IsProviderConnectionFromContext(ctx)), nil
}

func (s *Selector) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	ctx = interrupt.ContextWithIsExternalConnection(ctx)
	// lx: SPEC 064 — register the inbound conn. Upstream 515a73e4e now hands `s`
	// (not `selected`) to ConnectionManager, so the plain-outbound branch below
	// registers its outbound socket via Selector.DialContext on its own; the
	// handler branch (nested groups, handler outbounds) still bypasses it, and
	// wrapping before the branch keeps both covered. Approach from upstream PR #4285.
	conn = s.interruptGroup.NewConn(conn, true)
	selected := s.selected.Load()
	conn = s.interruptGroup.NewConn(conn, interrupt.IsExternalConnectionFromContext(ctx), interrupt.IsProviderConnectionFromContext(ctx))
	if outboundHandler, isHandler := selected.(adapter.ConnectionHandler); isHandler {
		outboundHandler.NewConnection(ctx, conn, metadata, onClose)
	} else {
		s.connection.NewConnection(ctx, s, conn, metadata, onClose)
	}
}

func (s *Selector) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	ctx = interrupt.ContextWithIsExternalConnection(ctx)
	// lx: SPEC 064 — see NewConnection; same registration gap on the UDP path.
	conn = s.interruptGroup.NewSingPacketConn(conn, true)
	selected := s.selected.Load()
	conn = s.interruptGroup.NewSingPacketConn(conn, interrupt.IsExternalConnectionFromContext(ctx), interrupt.IsProviderConnectionFromContext(ctx))
	if outboundHandler, isHandler := selected.(adapter.PacketConnectionHandler); isHandler {
		outboundHandler.NewPacketConnection(ctx, conn, metadata, onClose)
	} else {
		s.connection.NewPacketConnection(ctx, s, conn, metadata, onClose)
	}
}

func RealTag(outboundManager adapter.OutboundManager, detour adapter.Outbound) string {
	tag := detour.Tag()
	for {
		group, isGroup := detour.(adapter.OutboundGroup)
		if !isGroup {
			return tag
		}
		tag = group.Now()
		var loaded bool
		detour, loaded = outboundManager.Outbound(tag)
		if !loaded {
			return tag
		}
	}
}

func (s *Selector) onProviderUpdated(tag string) error {
	_, loaded := s.providers[tag]
	if !loaded {
		return E.New(s.Tag(), ": ", "outbound provider not found: ", tag)
	}
	var (
		tags          = s.Dependencies()
		outboundByTag = make(map[string]adapter.Outbound)
	)
	for _, tag := range tags {
		outboundByTag[tag] = s.outbounds[tag]
	}
	for _, providerTag := range s.providerTags {
		if providerTag != tag && s.outboundsCache[providerTag] != nil {
			for _, detour := range s.outboundsCache[providerTag] {
				tags = append(tags, detour.Tag())
				outboundByTag[detour.Tag()] = detour
			}
			continue
		}
		provider := s.providers[providerTag]
		var cache []adapter.Outbound
		for _, detour := range provider.Outbounds() {
			tag := detour.Tag()
			if s.exclude != nil && s.exclude.MatchString(tag) {
				continue
			}
			if s.include != nil && !s.include.MatchString(tag) {
				continue
			}
			tags = append(tags, tag)
			cache = append(cache, detour)
			outboundByTag[tag] = detour
		}
		s.outboundsCache[providerTag] = cache
	}
	if len(tags) == 0 {
		detour, _ := s.outbound.Outbound("Compatible")
		tags = append(tags, detour.Tag())
		outboundByTag[detour.Tag()] = detour
	}
	s.tags, s.outbounds = tags, outboundByTag
	detour, _ := s.outboundSelect()
	if s.selected.Swap(detour) != detour {
		s.interruptGroup.Interrupt(s.interruptExternalConnections)
	}
	return nil
}

func (s *Selector) outboundSelect() (adapter.Outbound, error) {
	if s.Tag() != "" {
		cacheFile := service.FromContext[adapter.CacheFile](s.ctx)
		if cacheFile != nil {
			selected := cacheFile.LoadSelected(s.Tag())
			if selected != "" {
				detour, loaded := s.outbounds[selected]
				if loaded {
					return detour, nil
				}
			}
		}
	}

	if s.defaultTag != "" {
		detour, loaded := s.outbounds[s.defaultTag]
		if !loaded {
			return nil, E.New("default outbound not found: ", s.defaultTag)
		}
		return detour, nil
	}

	return s.outbounds[s.tags[0]], nil
}
