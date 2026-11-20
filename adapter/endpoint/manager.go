package endpoint

import (
	"context"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/task"
)

var _ adapter.EndpointManager = (*Manager)(nil)

type Manager struct {
	registry        adapter.EndpointRegistry
	access          sync.Mutex
	scope           *adapter.Scope
	endpointScopes  map[string]*adapter.Scope
	endpointLoggers map[string]log.ContextLogger
	endpoints       []adapter.Endpoint
	endpointByTag   map[string]adapter.Endpoint
	// lx:begin chain
	// SPEC 073: опции созданных endpoint'ов по тегу — фабрика звеньев цепочки
	// пересоздаёт endpoint из них.
	optionsByTag map[string]managedOptions
	// lx:end chain
}

func NewManager(registry adapter.EndpointRegistry) *Manager {
	return &Manager{
		registry:        registry,
		endpointByTag:   make(map[string]adapter.Endpoint),
		endpointScopes:  make(map[string]*adapter.Scope),
		endpointLoggers: make(map[string]log.ContextLogger),
		optionsByTag:    make(map[string]managedOptions), // lx: chain
	}
}

func (m *Manager) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	m.access.Lock()
	defer m.access.Unlock()
	if stage == adapter.StartStateInitialize {
		m.scope = scope
		// lx: separate endpoint scopes keep upstream ownership while allowing
		// independent teardowns to run concurrently. The parent cancels them all
		// before this joined cleanup; no endpoint is abandoned on failure.
		scope.Add(m.closeEndpoints)
		for _, endpoint := range m.endpoints {
			m.endpointScopes[endpoint.Tag()] = adapter.NewScope(scope.Context(), m.endpointLoggers[endpoint.Tag()])
		}
	}
	if stage == adapter.StartStateStart {
		return nil
	}
	for _, endpoint := range m.endpoints {
		name := "endpoint/" + endpoint.Type() + "[" + endpoint.Tag() + "]"
		err := m.endpointScopes[endpoint.Tag()].Start(name, endpoint, stage)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) StartEndpoint(endpoint adapter.Endpoint) error {
	m.access.Lock()
	scope := m.endpointScopes[endpoint.Tag()]
	m.access.Unlock()
	if scope == nil {
		return E.New("endpoint manager is not initialized")
	}
	return scope.Start("endpoint/"+endpoint.Type()+"["+endpoint.Tag()+"]", endpoint, adapter.StartStateStart)
}

func (m *Manager) closeEndpoints() error {
	m.access.Lock()
	scopes := m.endpointScopes
	m.endpointScopes = nil
	m.access.Unlock()
	if len(scopes) == 0 {
		return nil
	}
	group := task.Group{}
	group.Concurrency(8)
	for tag, scope := range scopes {
		group.Append("endpoint["+tag+"]", func(context.Context) error {
			return scope.Close()
		})
	}
	return group.Run(context.Background())
}

func (m *Manager) Endpoints() []adapter.Endpoint {
	m.access.Lock()
	defer m.access.Unlock()
	return m.endpoints
}

func (m *Manager) Get(tag string) (adapter.Endpoint, bool) {
	m.access.Lock()
	defer m.access.Unlock()
	endpoint, found := m.endpointByTag[tag]
	return endpoint, found
}

func (m *Manager) Create(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, outboundType string, options any) error {
	endpoint, err := m.registry.Create(ctx, router, logger, tag, outboundType, options)
	if err != nil {
		return err
	}
	m.access.Lock()
	defer m.access.Unlock()
	_, loaded := m.endpointByTag[tag]
	if loaded {
		return E.New("duplicate endpoint tag: ", tag)
	}
	m.endpoints = append(m.endpoints, endpoint)
	m.endpointByTag[tag] = endpoint
	m.endpointLoggers[tag] = logger
	m.optionsByTag[tag] = managedOptions{endpointType: outboundType, options: options} // lx: chain
	return nil
}
