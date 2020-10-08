package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Config struct {
	// OpenVPNBinary is the absolute path to the patched OpenVPN binary.
	OpenVPNBinary string

	// OpenVPNConfigFile is the absolute path to the OpenVPN config file.
	OpenVPNConfigFile string

	// ServerAddress is the address on which to serve to receive the SAML
	// callback.
	ServerAddress string

	// ServerTimeout is the maximum amount of time to wait before closing the
	// server waiting for the SAML callback.
	ServerTimeout time.Duration

	// BrowserCommand is the command to run to open the SAML authorization URL.
	BrowserCommand []string

	// RedirectURL is an optional URL to redirect the user to after a
	// successful connection.
	RedirectURL string

	// RunCommand determines whether to run the command or to output the
	// command to stdout.
	RunCommand bool

	// TempCredentialsLocation is the location to save the temporary
	// credentials file.
	TempCredentialsLocation string

	// TempCredentialsPermissions is the permissions for the temp credentials
	// file.
	TempCredentialsPermissions uint
}

// DefaultCredsFilePath returns an absolute path to the default location for
// the credentials file.
func DefaultCredsFilePath() string {
	if cachedir, err := os.UserCacheDir(); err == nil {
		return path.Join(cachedir, "/samlvpn-credentials")
	}
	return path.Join(os.Getenv("HOME"), ".samlvpn-credentials")
}

func defaultConfig() *Config {
	return &Config{
		ServerAddress:              "127.0.0.1:35001",
		ServerTimeout:              time.Second * 60,
		BrowserCommand:             []string{"x-www-browser"},
		RunCommand:                 false,
		TempCredentialsLocation:    DefaultCredsFilePath(),
		TempCredentialsPermissions: 0400,
	}
}

func (c *Config) Parse(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if len(line) > 0 && line[0] == '#' {
			continue
		}

		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			return fmt.Errorf("could not parse config file, invalid number of parts on line %d", lineNumber)
		}

		switch parts[0] {
		case "openvpn-binary":
			c.OpenVPNBinary = os.ExpandEnv(strings.TrimSpace(parts[1]))

		case "openvpn-config-file":
			c.OpenVPNConfigFile = os.ExpandEnv(strings.TrimSpace(parts[1]))

		case "server-address":
			c.ServerAddress = strings.TrimSpace(parts[1])

		case "server-timeout":
			timeout, err := time.ParseDuration(strings.TrimSpace(parts[1]))
			if err != nil {
				return fmt.Errorf("could not parse server-timeout: %s", err)
			}
			c.ServerTimeout = timeout

		case "browser-command":
			c.BrowserCommand = parts[1:]

		case "redirect-url":
			c.RedirectURL = strings.TrimSpace(parts[1])

		case "run-command":
			c.RunCommand = strings.TrimSpace(parts[1]) == "true"

		case "temp-credentials-file-path":
			c.TempCredentialsLocation = os.ExpandEnv(strings.TrimSpace(parts[1]))

		case "temp-credentials-file-permissions":
			perms, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 8, 64)
			if err != nil {
				return fmt.Errorf("could not parse temp-credentials-file-permissions: %s", err)
			}
			c.TempCredentialsPermissions = uint(perms)
		}
	}

	return nil
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
