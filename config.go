package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Config struct {
	// OpenVPNBinary is the absolute path to the patched OpenVPN binary.
	OpenVPNBinary string `yaml:"openvpn-binary"`

	// OpenVPNConfigFile is the absolute path to the OpenVPN config file.
	OpenVPNConfigFile string `yaml:"openvpn-config-file"`

	// BrowserCommand is the format to run to open the SAML authorization URL.
	BrowserCommand []string `yaml:"browser-command"`

	// RedirectURL is an optional URL to redirect the user to after a
	// successful connection.
	RedirectURL string `yaml:"redirect-url"`

	// RunCommand determines whether to run the command or to output the
	// command to stdout.
	RunCommand bool `yaml:"run-command"`

	// Command to run on connection loss.
	ConnectionLostCommand []string `yaml:"connection-lost-command"`

	// Retries to run OpenVPN if the VPN returns AUTH_FAILED.
	AuthFailedRetries int `yaml:"auth-failed-retries"`

	// TempCredentialsFilePath is the location to save the temporary
	// credentials file.
	TempCredentialsFilePath string `yaml:"temp-credentials-file-path"`

	// TempCredentialsPermissions is the permissions for the temp credentials
	// file.
	TempCredentialsPermissions uint `yaml:"temp-credentials-permission"`
}

// DefaultCredsFilePath returns an absolute path to the default location for
// the credentials file.
func DefaultCredsFilePath() string {
	if cachedir, err := os.UserCacheDir(); err == nil {
		return path.Join(cachedir, "/samlvpn-credentials")
	}
	return path.Join(os.Getenv("HOME"), ".samlvpn-credentials")
}

// ParseWithDefaults parses the contents of r into c. It also sets defaults for
// optionals if the parsed file didn't override them.
func (c *Config) ParseWithDefaults(r io.Reader) error {
	if err := yaml.NewDecoder(r).Decode(&c); err != nil {
		return errors.Wrap(err, "could not decode configuration file")
	}

	if c.TempCredentialsFilePath == "" {
		c.TempCredentialsFilePath = DefaultCredsFilePath()
	}
	if c.TempCredentialsPermissions == 0 {
		c.TempCredentialsPermissions = 0400
	}

	c.OpenVPNBinary = os.ExpandEnv(c.OpenVPNBinary)
	c.OpenVPNConfigFile = os.ExpandEnv(c.OpenVPNConfigFile)
	c.TempCredentialsFilePath = os.ExpandEnv(c.TempCredentialsFilePath)

	return nil
}

// Validate returns errors regarding the configuration.
func (c *Config) Validate() []error {
	var errs []error

	if c.OpenVPNBinary == "" {
		errs = append(errs, errors.Errorf("openvpn-binary is required"))
	}
	if _, err := os.Stat(os.ExpandEnv(c.OpenVPNBinary)); err != nil {
		errs = append(errs, errors.Wrap(err, "could not stat openvpn-binary"))
	}

	if c.OpenVPNConfigFile == "" {
		errs = append(errs, errors.Errorf("openvpn-config-file is required"))
	}
	if _, err := os.Stat(os.ExpandEnv(c.OpenVPNConfigFile)); err != nil {
		errs = append(errs, errors.Wrap(err, "could not stat openvpn-config-file"))
	}

	var hasFmtSpec bool
	if len(c.BrowserCommand) != 0 {
		for i := range c.BrowserCommand {
			if c.BrowserCommand[i] == "%s" {
				hasFmtSpec = true
				break
			}
		}
	}
	if !hasFmtSpec {
		errs = append(errs, errors.New("the browser-command must contain %s"))
	}

	return errs
}

type OpenVPNConfig struct {
	Host     string
	Port     int
	Protocol string
}

func ParseOpenVPNConfig(r io.Reader) (*OpenVPNConfig, error) {
	config := &OpenVPNConfig{}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			continue
		}

		switch parts[0] {
		case "remote":
			if len(parts[1:]) != 2 {
				return nil, fmt.Errorf("remote line does not include host and port")
			}
			config.Host = parts[1]
			port, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return nil, errors.Wrap(err, "remote line has non-integer port")
			}
			config.Port = int(port)

		case "proto":
			config.Protocol = parts[1]
		}
	}

	return config, nil
}
