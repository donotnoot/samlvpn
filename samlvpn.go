package main

import (
	"bufio"
	"bytes"
	"context"
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
	ErrAuthFailed     = errors.New("auth failed")
	ErrConnectionLost = errors.New("connection established then lost")
)

// SAMLVPN makes it easy to retry stuff that fails, etc.
type SAMLVPN struct {
	Config        *Config
	OpenVPNConfig *OpenVPNConfig
}

// Configure parses all configs and sets them in s.
func (s *SAMLVPN) Configure(flagConfigFile *string) error {
	var configFilePath string
	defaultConfigFiles := []string{
		"$XDG_CONFIG_HOME/samlvpn/config.yaml",
		"$XDG_CONFIG_HOME/samlvpn.yaml",
		"$HOME/.config/samlvpn.yaml",
		"$HOME/.samlvpn.yaml",
	}

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

	if configFilePath == "" {
		return errors.Errorf(
			"please specify a config file, could not find any default in %q",
			strings.Join(defaultConfigFiles, ", "))
	}

	configFile, err := os.Open(configFilePath)
	if err != nil {
		return errors.Wrap(err, "could not open config file")
	}
	defer configFile.Close()

	s.Config = &Config{}

	if err := s.Config.ParseWithDefaults(configFile); err != nil {
		return errors.Wrapf(err, "could not parse %q", configFilePath)
	}
	log.Printf("parsed config file %q", configFilePath)

	if errs := s.Config.Validate(); errs != nil {
		log.Println("the configuration file contains the following error(s):")
		for i := range errs {
			fmt.Fprintln(os.Stderr, "-", errs[i].Error())
		}
		os.Exit(1)
	}

	openVPNConfigFile, err := os.Open(s.Config.OpenVPNConfigFile)
	if err != nil {
		return errors.Wrap(err, "could not open OpenVPN config")
	}
	defer openVPNConfigFile.Close()

	s.OpenVPNConfig, err = ParseOpenVPNConfig(openVPNConfigFile)
	if err != nil {
		errors.Wrap(err, "could not parse OpenVPN config")
	}

	return nil
}

// resolveHostname resolves the OpenVPN hostname.
func (s *SAMLVPN) resolveHostname() (string, error) {
	host := randomString() + "." + s.OpenVPNConfig.Host
	log.Println("looking up", host)
	addrs, err := net.LookupHost(host)
	if err != nil {
		return "", err
	}
	if len(addrs) < 1 {
		return "", errors.Errorf("no addresses found")
	}
	return addrs[0], nil
}

func (s *SAMLVPN) getLoginURLAndSID() (*url.URL, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	authFile, err := tmpfile(s.Config.TempCredentialsFilePath,
		"N/A\nACS::35001", s.Config.TempCredentialsPermissions)
	if err != nil {
		return nil, "", errors.Wrap(err, "could not create temp file")
	}
	defer authFile.Close()

	cmd := exec.CommandContext(
		ctx,
		s.Config.OpenVPNBinary,
		"--config", s.Config.OpenVPNConfigFile,
		"--verb", "3",
		"--auth-retry", "none",
		"--auth-user-pass", authFile.Name(),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, "", errors.Errorf(
			"could not run command to get AUTH_FAILED response: %s\nOpenVPN output:\n%s",
			err, string(output))
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "AUTH_FAILED") {
			split := strings.Split(line, ":")
			if len(split) < 10 {
				return nil, "", errors.Errorf("could not find SID in output: %q", line)
			}
			url, err := url.Parse(split[8] + ":" + split[9])
			if err != nil {
				return nil, "", errors.Wrap(err, "could not parse URL")
			}
			return url, split[6], nil
		}
	}

	return nil, "", errors.Errorf("could not find AUTH_FAILED line")
}

func (s *SAMLVPN) getSAMLCallback(challengeURL string) (string, error) {
	addr := "0.0.0.0:35001"
	// Long timeout to allow user to follow SAML prompts etc.
	timeout := time.Second * 120
	log.Printf("starting HTTP server on %s, timeout %v", addr, timeout)

	server := NewServer(addr, s.Config.RedirectURL, timeout)
	server.Start()

	s.openOrShowLink(challengeURL)

	log.Println("waiting for server to receive SAML callback")
	response, err := server.WaitForResponse()
	if err != nil {
		return "", errors.Wrap(err, "could not get response")
	}

	return response, nil
}

// openOrShowLink opens or shows a link to the user, depending on config.
func (s *SAMLVPN) openOrShowLink(url string) {
	if len(s.Config.BrowserCommand) < 1 {
		log.Println("open this:", url)
		return
	}

	for i := range s.Config.BrowserCommand {
		if strings.Contains(s.Config.BrowserCommand[i], "%s") {
			s.Config.BrowserCommand[i] = fmt.Sprintf(s.Config.BrowserCommand[i], url)
			break
		}
	}
	cmd := exec.Command(s.Config.BrowserCommand[0], s.Config.BrowserCommand[1:]...)
	log.Println("launching", cmd)
	output := &bytes.Buffer{}
	cmd.Stderr = output
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		log.Println(errors.Wrap(err, "could not open URL in browser"))
		log.Println("open this manually:", url)
		return
	}
	log.Println("your browser said:", strings.TrimSpace(output.String()))
}

func (s *SAMLVPN) getCredentials() (string, error) {
	challengeURL, SID, err := s.getLoginURLAndSID()
	if err != nil {
		return "", errors.Wrap(err, "could not get challenge URL")
	}

	samlCallback, err := s.getSAMLCallback(challengeURL.String())
	if err != nil {
		return "", errors.Wrap(err, "could not get SAML callback")
	}

	return fmt.Sprintf("N/A\nCRV1::%s::%s", SID, samlCallback), nil
}

func (s *SAMLVPN) Connect() error {
	credentials, err := s.getCredentials()
	if err != nil {
		return errors.Wrap(err, "could not get credentials")
	}

	credsFile, err := tmpfile(s.Config.TempCredentialsFilePath,
		credentials, s.Config.TempCredentialsPermissions)
	if err != nil {
		log.Fatal(errors.Wrap(err, "could not create credentials file"))
	}
	defer credsFile.Close()
	log.Println("saved credentials to", credsFile.Name())

	if s.Config.RunCommand {
		for i := 0; i <= s.Config.AuthFailedRetries; i++ {
			err := s.runCommand(credsFile)
			if errors.Is(err, ErrAuthFailed) && s.Config.AuthFailedRetries != 0 {
				log.Println("Auth failed, try #", i+1, ".")
				continue
			}
			if errors.Is(err, ErrConnectionLost) {
				log.Println("Connection lost. Restart SamlVPN to reconnect.")
				s.runConnLost()
			}
			return err
		}
	}

	s.printCommand(credsFile)

	return nil
}

// runConnLost runs the command that the user wants to run when the connection
// is lost.
func (s *SAMLVPN) runConnLost() {
	if len(s.Config.ConnectionLostCommand) == 0 {
		return
	}

	cmd := exec.Command(
		s.Config.ConnectionLostCommand[0],
		s.Config.ConnectionLostCommand[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		log.Printf("conn lost command did not execute correctly: %s", err)
	}
}

// rebuildCommand rebuilds the OpenVPN command with a new hostname.
func (s *SAMLVPN) rebuildCommand(ctx context.Context, credsFile *os.File) (*exec.Cmd, error) {
	cmd := exec.CommandContext(
		ctx,
		"sudo",
		s.Config.OpenVPNBinary,
		"--config", s.Config.OpenVPNConfigFile,
		"--verb", "3",
		"--auth-nocache",
		"--proto", s.OpenVPNConfig.Protocol,
		"--auth-retry", "none",
		"--auth-user-pass", credsFile.Name())

	hostname, err := s.resolveHostname()
	if err != nil {
		return nil, errors.Wrap(err, "could not resolve hostname")
	}
	cmd.Args = append(cmd.Args, "--remote", hostname, fmt.Sprint(s.OpenVPNConfig.Port))

	return cmd, nil
}

func (s *SAMLVPN) runCommand(credsFile *os.File) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd, err := s.rebuildCommand(ctx, credsFile)
	if err != nil {
		return errors.Wrap(err, "could not rebuild command")
	}

	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(&output, os.Stdout)
	cmd.Stderr = io.MultiWriter(&output, os.Stderr)

	log.Println("starting openvpn")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "could not run command")
	}

	// Check if it failed to run. The exit code does not indicate this so need
	// to check the logs.
	sca := bufio.NewScanner(&output)
	for sca.Scan() {
		line := sca.Text()

		if strings.Contains(line, "AUTH_FAILED") {
			return ErrAuthFailed
		}
		if strings.Contains(line, "Initialization Sequence Completed") {
			// After the init sequence is complete, the failure can only
			// logically be that the connection has been lost.
			return ErrConnectionLost
		}
	}
	if err := sca.Err(); err != nil {
		return errors.Wrap(err, "could not close scanner")
	}

	return nil
}

func (s *SAMLVPN) printCommand(credsFile *os.File) error {
	cmd, err := s.rebuildCommand(context.TODO(), credsFile)
	if err != nil {
		return errors.Wrap(err, "could not rebuild command")
	}

	fmt.Print(cmd)

	return nil
}
