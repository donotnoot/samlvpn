.PHONY: patch

GOFILES += main.go
GOFILES += config.go
GOFILES += server.go

bin: $(GOFILES)
	go build -o bin/samlvpn $(GOFILES)

install: $(GOFILES)
	go install

clean:
	rm -rf bin

config.go:
	cp config.go.example config.go

patch:
	git apply --directory openvpn openvpn-v2.4.9.diff

