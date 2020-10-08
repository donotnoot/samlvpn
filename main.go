package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var (
	flagConfigFile = flag.String("config", "", "config file")

	defaultConfigFiles = []string{
		"$HOME/.samlvpn",
		"$HOME/.config/samlvpn",
		"$XDG_CONFIG_HOME/.samlvpn",
	}
)

func config() (*Config, error) {
	var configFilePath string
	if *flagConfigFile != "" {
		configFilePath = *flagConfigFile
	} else {
		for _, path := range defaultConfigFiles {
			fullPath := os.ExpandEnv(path)
			if _, err := os.Stat(fullPath); err == nil {
				configFilePath = fullPath
				break
			}
		}
	}

	if configFilePath != "" {
		config := defaultConfig()

		file, err := os.Open(configFilePath)
		if err != nil {
			return nil, errors.Wrap(err, "could not open config file")
		}
		defer file.Close()

		if err := config.Parse(file); err != nil {
			return nil, errors.Wrapf(err, "could not parse %q", configFilePath)
		}
		log.Printf("parsed config file %q", configFilePath)

		return config, nil
	}

	return nil, fmt.Errorf("please specify a config file")

}

func openVPNConfig(path string) (*OpenVPNConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "could not open OpenVPN config")
	}
	defer file.Close()

	return ParseOpenVPNConfig(file)
}

func main() {
	flag.Parse()
	config, err := config()
	if err != nil {
		log.Fatal(err)
	}

	openVPNConfig, err := openVPNConfig(config.OpenVPNConfigFile)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("parsed OpenVPN config file")

	log.Println("resolving VPN hostname")
	vpnRemote, err := vpnIPAddress(openVPNConfig.Host)
	if err != nil {
		log.Fatal(errors.Wrap(err, "could not resolve VPN hostname"))
	}
	log.Println("IP Address:", vpnRemote)

	log.Println("obtaining AUTH_FAILED response")
	output, err := samlAuthErrorLogOutput(config, openVPNConfig)
	if err != nil {
		log.Fatal(errors.Wrapf(err,
			"could not get AUTH_FAILED response, got\n%s", output))
	}

	log.Println("parsing AUTH_FAILED response")
	URL, SID, err := parseOutput(output)
	if err != nil {
		log.Fatal(errors.Wrap(err, "could not parse challenge URL"))
	}

	log.Printf("starting HTTP server on %s, timeout %v", config.ServerAddress, config.ServerTimeout)
	server := NewServer(config.ServerAddress, config.RedirectURL, config.ServerTimeout)
	server.Start()

	if len(config.BrowserCommand) == 0 {
		log.Println("open this:", URL)
	} else {
		cmd := exec.Command(
			config.BrowserCommand[0],
			append(config.BrowserCommand[1:], URL.String())...,
		)
		log.Println("launching", cmd)
		output := &bytes.Buffer{}
		cmd.Stderr = output
		cmd.Stdout = output
		if err := cmd.Run(); err != nil {
			log.Println(errors.Wrap(err, "could not open URL in browser"))
			log.Println("open this manually:", URL.String())
		}
		log.Println("your browser said:", strings.TrimSpace(output.String()))
	}

	log.Println("waiting for server to receive SAML callback")
	response, err := server.WaitForResponse()
	if err != nil {
		log.Println(errors.Wrap(err, "could not get response"))
	}

	credentials := fmt.Sprintf("N/A\nCRV1::%s::%s", SID, response)

	cmd := exec.Command(
		"sudo",
		config.OpenVPNBinary,
		"--config", config.OpenVPNConfigFile,
		"--verb", "3",
		"--auth-nocache",
		"--inactive", "3600",
		"--proto", openVPNConfig.Protocol,
		"--remote", vpnRemote, fmt.Sprint(openVPNConfig.Port),
	)

	if config.RunCommand {
		cmd.Args = append(cmd.Args, "--auth-user-pass", "/dev/stdin")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = bytes.NewBufferString(credentials)

		if err := cmd.Run(); err != nil {
			log.Println(err)
		}

		return
	}

	credsFile, err := tmpfile(config, credentials)
	if err != nil {
		log.Fatal(errors.Wrap(err, "could not create credentials file"))
	}
	defer credsFile.Close()
	log.Println("saved credentials to", credsFile.Name())

	cmd.Args = append(cmd.Args, "--auth-user-pass", credsFile.Name())

	fmt.Print(cmd.String())
}

func samlAuthErrorLogOutput(config *Config, ovpnConfig *OpenVPNConfig) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		config.OpenVPNBinary,
		"--config", config.OpenVPNConfigFile,
		"--verb", "3",
		"--proto", ovpnConfig.Protocol,
		"--remote", ovpnConfig.Host, fmt.Sprint(ovpnConfig.Port),
		"--auth-retry", "none",
		"--auth-user-pass", "/dev/stdin",
	)
	output := &bytes.Buffer{}
	cmd.Stdin = bytes.NewBufferString("N/A\nACS::35001")
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return output.String(), nil
}

func randomString() string {
	b := make([]byte, 12)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	return hex.EncodeToString(b)
}

func vpnIPAddress(hostname string) (string, error) {
	host := randomString() + "." + hostname
	log.Println("looking up", host)
	addrs, err := net.LookupHost(host)
	if err != nil {
		return "", errors.Wrap(err, "could not lookup host")
	}
	if len(addrs) < 1 {
		return "", fmt.Errorf("could not lookup host: no addresses found")
	}
	return addrs[0], nil
}

func tmpfile(config *Config, contents string) (*os.File, error) {
	if _, err := os.Stat(config.TempCredentialsLocation); err == nil {
		err := os.Remove(config.TempCredentialsLocation)
		if err != nil {
			return nil, errors.Wrap(err, "could not delete old file")
		}
	}

	file, err := os.OpenFile(
		config.TempCredentialsLocation,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		os.FileMode(config.TempCredentialsPermissions))
	if err != nil {
		return nil, errors.Wrap(err, "could not create credentials file")
	}

	if _, err := io.WriteString(file, contents); err != nil {
		return nil, errors.Wrap(err, "could not write temp file contents")
	}

	return file, nil
}

// parseOutput crudely gets the SAML URL and the SID from the logs output.
func parseOutput(output string) (*url.URL, string, error) {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "AUTH_FAILED") {
			split := strings.Split(line, ":")
			if len(split) < 10 {
				return nil, "", fmt.Errorf("could not find SID in output: %q", line)
			}
			url, err := url.Parse(split[8] + ":" + split[9])
			if err != nil {
				return nil, "", errors.Wrap(err, "could not parse URL")
			}
			return url, split[6], nil
		}
	}

	return nil, "", fmt.Errorf("could not find AUTH_FAILED line")
}
