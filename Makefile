.PHONY: all test clean
all: diskatlas

diskatlas: $(wildcard *.go) go.mod go.sum
	go build -o diskatlas .

test:
	go test ./...
	go vet ./...

clean:
	rm -f diskatlas
