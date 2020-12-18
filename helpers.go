package main

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"

	"github.com/pkg/errors"
)

func randomString() string {
	b := make([]byte, 12)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	return hex.EncodeToString(b)
}

func tmpfile(path, contents string, perms uint) (*os.File, error) {
	if _, err := os.Stat(path); err == nil {
		err := os.Remove(path)
		if err != nil {
			return nil, errors.Wrap(err, "could not delete old file")
		}
	}

	file, err := os.OpenFile(
		os.ExpandEnv(path),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		os.FileMode(perms))
	if err != nil {
		return nil, errors.Wrap(err, "could not create credentials file")
	}

	if _, err := io.WriteString(file, contents); err != nil {
		return nil, errors.Wrap(err, "could not write temp file contents")
	}

	return file, nil
}
