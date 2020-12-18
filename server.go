package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

func samlSuccessHTML(redirectURL string) string {
	var redirectHTML string
	var message string = "You can close this now"
	if redirectURL != "" {
		redirectHTML = fmt.Sprintf(`
			<meta http-equiv="refresh" content="5; url=%s" />`, redirectURL)
		message = fmt.Sprintf("Redirecting you to %s...", redirectURL)
	}
	return fmt.Sprintf(`
<html>
	<head>
		<title>SamlVPN</title>
		%s
	</head>
	<body>
		<h2>Got SAML response!</h2>
		<p>%s</p>
		<br>
		<small>
			Thank you for using <a href="github.com/donotnoot/samlvpn">SamlVPN</a>!
		</small>
	</body>
</html>`, redirectHTML, message)
}

type Server struct {
	httpServer *http.Server
	response   chan string
	timeout    time.Duration
}

func NewServer(address, redirectURL string, timeout time.Duration) *Server {
	response := make(chan string)

	return &Server{
		timeout:  timeout,
		response: response,
		httpServer: &http.Server{
			Addr:              address,
			ReadTimeout:       time.Second,
			IdleTimeout:       time.Second,
			WriteTimeout:      time.Second,
			ReadHeaderTimeout: time.Second,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				log.Println("handling HTTP request", r.Method, r.URL)
				defer r.Body.Close()

				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					w.Write([]byte("hey there! you might want to try POST"))
					return
				}

				if err := r.ParseForm(); err != nil {
					err := errors.Wrap(err, "could not parse SAML form data")
					log.Println(err)
					writeError(w, err)
					return
				}
				samlResponse := r.FormValue("SAMLResponse")
				if len(samlResponse) == 0 {
					err := fmt.Errorf("SAMLResponse from field has zero length")
					log.Println(err)
					writeError(w, err)
					return
				}
				response <- samlResponse

				w.WriteHeader(200)
				w.Write([]byte(samlSuccessHTML(redirectURL)))
			}),
		},
	}
}

func writeError(w http.ResponseWriter, err error) {
	w.WriteHeader(500)
	w.Write([]byte(fmt.Sprint(err)))
}

func (s *Server) Start() {
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

}

func (s *Server) WaitForResponse() (string, error) {
	defer func() {
		if err := s.httpServer.Close(); err != nil {
			log.Fatal(errors.Wrap(err, "could not close server"))
		}
	}()

	select {
	case response := <-s.response:
		return response, nil

	case <-time.After(s.timeout):
		return "", fmt.Errorf("timed out waiting for response after %v", s.timeout)
	}
}
