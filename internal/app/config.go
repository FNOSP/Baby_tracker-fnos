package app

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultGatewayPrefix = "/app/baby-tracker"

// Config contains the runtime paths and listener settings used by the server.
type Config struct {
	DataDir            string
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
	prefix := strings.TrimSpace(os.Getenv("BABY_TRACKER_GATEWAY_PREFIX"))
	if prefix == "" {
		prefix = defaultGatewayPrefix
	}
	prefix = "/" + strings.Trim(prefix, "/")

	return Config{
		DataDir:            dataDir,
		WebDir:             webDir,
		UnixSocket:         strings.TrimSpace(os.Getenv("BABY_TRACKER_GATEWAY_SOCKET")),
		ListenAddress:      strings.TrimSpace(os.Getenv("BABY_TRACKER_LISTEN_ADDRESS")),
		GatewayPrefix:      prefix,
		SessionSecret:      strings.TrimSpace(os.Getenv("SECRET_KEY")),
		EtcDir:             strings.TrimSpace(os.Getenv("TRIM_PKGETC")),
		LauncherPath:       strings.TrimSpace(os.Getenv("BABY_TRACKER_CMD_MAIN")),
		CreateDefaultAdmin: envBool("BABY_TRACKER_CREATE_DEFAULT_ADMIN"),
	}
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
