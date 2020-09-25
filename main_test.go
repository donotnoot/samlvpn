package main

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mustParseURL(u string) *url.URL {
	url, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	return url
}

func TestParseOutput(t *testing.T) {
	for name, tc := range map[string]struct {
		logOutput string
		URL       *url.URL
		SID       string
		err       assert.ErrorAssertionFunc
	}{
		"Good output": {
			logOutput: `
Fri Sep 25 13:12:53 2020 Some other log line :)
Fri Sep 25 13:12:53 2020 AUTH: Received control message: AUTH_FAILED,CRV1:R:instance-1/6876397182473095132/690502db-7813-4267-9706-be0838081823:b'Ti9B':https://samlwebsite.com/app/clientvpn/someURL
Fri Sep 25 13:12:53 2020 Just OpenVPN things`,
			URL: mustParseURL("https://samlwebsite.com/app/clientvpn/someURL"),
			SID: "instance-1/6876397182473095132/690502db-7813-4267-9706-be0838081823",
			err: assert.NoError,
		},
		"Bad output": {
			logOutput: `Completely bogus!`,
			URL:       nil,
			SID:       "",
			err:       assert.Error,
		},
	} {
		t.Run(name, func(t *testing.T) {
			URL, SID, err := parseOutput(tc.logOutput)
			assert.Equal(t, tc.URL, URL)
			assert.Equal(t, tc.SID, SID)
			tc.err(t, err)
		})
	}
}
