.PHONY: all test clean
all: diskatlas

diskatlas: main.go go.mod go.sum
	go build -o diskatlas .

test:
	go test ./...
	go vet ./...

clean:
	rm -f diskatlas
