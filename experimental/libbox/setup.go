package libbox

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/sagernet/sing-box/common/networkquality"
	"github.com/sagernet/sing-box/common/stun"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/experimental/locale"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/service/oomkiller"
	"github.com/sagernet/sing/common/byteformats"
	E "github.com/sagernet/sing/common/exceptions"
)

var (
	sBasePath                string
	sWorkingPath             string
	sTempPath                string
	sUserID                  int
	sGroupID                 int
	sFixAndroidStack         bool
	sCommandServerListenPort uint16
	sCommandServerSecret     string
	sLogMaxLines             int
	sDebug                   bool
	sCrashReportSource       string
	sAppVersion              string
	sAppMarketingVersion     string
	sOOMKillerEnabled        bool
	sOOMKillerDisabled       bool
	sOOMMemoryLimit          int64
	sPowerReportEnabled      bool
	sPlatformMetadata        []byte
)

func init() {
	debug.SetPanicOnFault(true)
	debug.SetTraceback("all")
}

type SetupOptions struct {
	BasePath                string
	WorkingPath             string
	TempPath                string
	FixAndroidStack         bool
	CommandServerListenPort int32
	CommandServerSecret     string
	LogMaxLines             int
	Debug                   bool
	CrashReportSource       string
	AppVersion              string
	AppMarketingVersion     string
	OomKillerEnabled        bool
	OomKillerDisabled       bool
	OomMemoryLimit          int64
	PowerReportEnabled      bool
	PlatformMetadata        string
}

func applySetupOptions(options *SetupOptions) {
	sBasePath = options.BasePath
	sWorkingPath = options.WorkingPath
	sTempPath = options.TempPath
	C.AddResourcePath(sBasePath)

	sUserID = os.Getuid()
	sGroupID = os.Getgid()

	// TODO: remove after fixed
	// https://github.com/golang/go/issues/68760
	sFixAndroidStack = options.FixAndroidStack

	sCommandServerListenPort = uint16(options.CommandServerListenPort)
	sCommandServerSecret = options.CommandServerSecret
	sLogMaxLines = options.LogMaxLines
	sDebug = options.Debug
	sCrashReportSource = options.CrashReportSource
	sAppVersion = options.AppVersion
	sAppMarketingVersion = options.AppMarketingVersion
	ReloadSetupOptions(options)
}

func ReloadSetupOptions(options *SetupOptions) {
	sOOMKillerEnabled = options.OomKillerEnabled
	sOOMKillerDisabled = options.OomKillerDisabled
	sOOMMemoryLimit = options.OomMemoryLimit
	sPowerReportEnabled = options.PowerReportEnabled
	if json.Valid([]byte(options.PlatformMetadata)) {
		sPlatformMetadata = []byte(options.PlatformMetadata)
	} else {
		sPlatformMetadata = nil
	}
	if sOOMKillerEnabled {
		if sOOMMemoryLimit == 0 && C.IsIos {
			sOOMMemoryLimit = oomkiller.DefaultAppleNetworkExtensionMemoryLimit
			debug.SetGCPercent(oomkiller.DefaultAppleNetworkExtensionGCPercent)
		}
		if sOOMMemoryLimit > 0 {
			debug.SetMemoryLimit(int64(oomkiller.RuntimeMemoryLimit(uint64(sOOMMemoryLimit))))
		} else {
			debug.SetMemoryLimit(math.MaxInt64)
		}
	} else {
		debug.SetMemoryLimit(math.MaxInt64)
	}
}

func Setup(options *SetupOptions) error {
	applySetupOptions(options)
	os.MkdirAll(sWorkingPath, 0o777)
	os.MkdirAll(sTempPath, 0o777)
	err := redirectStderr(filepath.Join(sWorkingPath, "CrashReport-"+sCrashReportSource+".log"))
	savePlatformSnapshot()
	return err
}

func SetLocale(localeID string) error {
	if !locale.Set(localeID) {
		return E.New("unsupported locale: ", localeID)
	}
	return nil
}

func Version() string {
	return C.Version
}

func GoVersion() string {
	return runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH
}

func FormatBytes(length int64) string {
	return byteformats.FormatKBytes(uint64(length))
}

func FormatMemoryBytes(length int64) string {
	return byteformats.FormatMemoryKBytes(uint64(length))
}

func FormatDuration(duration int64) string {
	return log.FormatDuration(time.Duration(duration) * time.Millisecond)
}

func FormatBitrate(bps int64) string {
	return networkquality.FormatBitrate(bps).String()
}

func FormatBitrateString(bps int64) string {
	return networkquality.FormatBitrate(bps).String()
}

func FormatNATType(natType int32) string {
	return stun.NATType(natType).String()
}
