package app

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultGatewayPrefix = "/app/baby-tracker"
	defaultListenAddress = "0.0.0.0:5000"
)

// Config contains the runtime paths and listener settings used by the server.
type Config struct {
	DataDir            string
	LegacyDataDirs     []string
	WebDir             string
	UnixSocket         string
	ListenAddress      string
	GatewayPrefix      string
	SessionSecret      string
	EtcDir             string
	LauncherPath       string
	CreateDefaultAdmin bool
}

// LoadConfig reads the service configuration from environment variables.
func LoadConfig() Config {
	webDir := strings.TrimSpace(os.Getenv("BABY_TRACKER_WEB_DIR"))
	if webDir == "" {
		webDir = "."
	}
	dataDir := strings.TrimSpace(os.Getenv("BABY_TRACKER_DATA_DIR"))
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("TRIM_PKGVAR"))
	}
	if dataDir == "" {
		dataDir = filepath.Join(".", "data")
	}
	var legacyDataDirs []string
	if legacyDataDir := strings.TrimSpace(os.Getenv("BABY_TRACKER_LEGACY_DATA_DIR")); legacyDataDir != "" {
		legacyDataDirs = append(legacyDataDirs, legacyDataDir)
	}
	unixSocket := strings.TrimSpace(os.Getenv("BABY_TRACKER_GATEWAY_SOCKET"))
	listenAddress := strings.TrimSpace(os.Getenv("BABY_TRACKER_LISTEN_ADDRESS"))
	if listenAddress == "" && unixSocket == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			listenAddress = ":" + port
		} else {
			listenAddress = defaultListenAddress
		}
	}
	basePath := normalizePath(os.Getenv("BABY_TRACKER_BASE_PATH"))
	if basePath == "/" {
		basePath = ""
	}
	prefix := strings.TrimSpace(os.Getenv("BABY_TRACKER_GATEWAY_PREFIX"))
	if prefix == "" {
		prefix = basePath
	}
	if prefix == "" {
		prefix = defaultGatewayPrefix
	}
	prefix = normalizeGatewayPrefix(prefix)

	return Config{
		DataDir:            dataDir,
		LegacyDataDirs:     legacyDataDirs,
		WebDir:             webDir,
		UnixSocket:         unixSocket,
		ListenAddress:      listenAddress,
		GatewayPrefix:      prefix,
		SessionSecret:      strings.TrimSpace(os.Getenv("SECRET_KEY")),
		EtcDir:             strings.TrimSpace(os.Getenv("TRIM_PKGETC")),
		LauncherPath:       strings.TrimSpace(os.Getenv("BABY_TRACKER_CMD_MAIN")),
		CreateDefaultAdmin: envBool("BABY_TRACKER_CREATE_DEFAULT_ADMIN"),
	}
}

func normalizeGatewayPrefix(value string) string {
	prefix := strings.TrimRight(normalizePath(value), "/")
	if prefix == "" {
		return defaultGatewayPrefix
	}
	return prefix
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
