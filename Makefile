APP := ais
DOCS_DIR := docs/generated
GRUBY_DIR := vendors/gruby

.PHONY: all build gruby clean test run docs

all: build

build: gruby
	go build ./cmd/ais

gruby:
	$(MAKE) -C $(GRUBY_DIR) libmruby.a test

test: gruby
	go test ./...

run: gruby
	go run ./cmd/ais start -c configs/ais.rb

docs:
	rm -rf $(GRUBY_DIR)
	yard doc
	python -m http.server 8081 -d docs/generated

clean:
	rm -f $(APP)
	$(MAKE) -C $(GRUBY_DIR) clean
	go clean