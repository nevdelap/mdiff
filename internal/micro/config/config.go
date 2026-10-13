package config

import (
	"errors"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
)

var ConfigDir string

// DisableBufferStateWrites can suppress Micro's optional cursor/undo state
// serialization. mdiff leaves it off and resolves duplicate paths per buffer.
var DisableBufferStateWrites bool

// Diagnostics contains non-fatal configuration issues collected during
// startup. mdiff surfaces these in its status area instead of using Micro's
// blocking pre-screen prompt, which would be unsafe during CLI startup.
var Diagnostics []string

func AddDiagnostic(message string) { Diagnostics = append(Diagnostics, message) }

// InitConfigDir finds the configuration directory for micro according to the XDG spec.
// If no directory is found, it creates one.
func InitConfigDir(flagConfigDir string) error {
	var e error

	microHome := os.Getenv("MICRO_CONFIG_HOME")
	if microHome == "" {
		// The user has not set $MICRO_CONFIG_HOME so we'll try $XDG_CONFIG_HOME
		xdgHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgHome == "" {
			// The user has not set $XDG_CONFIG_HOME so we should act like it was set to ~/.config
			home, err := homedir.Dir()
			if err != nil {
				return errors.New("Error finding your home directory\nCan't load config files: " + err.Error())
			}
			xdgHome = filepath.Join(home, ".config")
		}

		microHome = filepath.Join(xdgHome, "micro")
	}
	ConfigDir = microHome

	if len(flagConfigDir) > 0 {
		if _, err := os.Stat(flagConfigDir); os.IsNotExist(err) {
			e = errors.New("Error: " + flagConfigDir + " does not exist. Defaulting to " + ConfigDir + ".")
		} else {
			ConfigDir = flagConfigDir
			return nil
		}
	}

	// Create micro config home directory if it does not exist
	// This creates parent directories and does nothing if it already exists
	err := os.MkdirAll(ConfigDir, os.ModePerm)
	if err != nil {
		return errors.New("Error creating configuration directory: " + err.Error())
	}

	return e
}
