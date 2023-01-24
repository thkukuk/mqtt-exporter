MQTT_EXPORTER_BIN := bin/mqtt-exporter

GO ?= go
GO_MD2MAN ?= go-md2man

VERSION := $(shell cat VERSION)
USE_VENDOR =
LOCAL_LDFLAGS = -buildmode=pie -ldflags "-X=github.com/thkukuk/mqtt-exporter/pkg/mqtt-exporter.Version=$(VERSION)"

.PHONY: all api build vendor
all: dep build

dep: ## Get the dependencies
	@$(GO) get -v -d ./...

update: ## Get and update the dependencies
	@$(GO) get -v -d -u ./...

tidy: ## Clean up dependencies
	@$(GO) mod tidy

vendor: dep ## Create vendor directory
	@$(GO) mod vendor

build: ## Build the binary files
	$(GO) build -v -o $(MQTT_EXPORTER_BIN) $(USE_VENDOR) $(LOCAL_LDFLAGS) ./cmd/mqtt-exporter

clean: ## Remove previous builds
	@rm -f $(MQTT_EXPORTER_BIN)

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'


.PHONY: release
release: ## create release package from git
	git clone https://github.com/thkukuk/mqtt-exporter
	mv mqtt-exporter mqtt-exporter-$(VERSION)
	sed -i -e 's|USE_VENDOR =|USE_VENDOR = -mod vendor|g' mqtt-exporter-$(VERSION)/Makefile
	make -C mqtt-exporter-$(VERSION) vendor
	cp VERSION mqtt-exporter-$(VERSION)
	tar --exclude .git -cJf mqtt-exporter-$(VERSION).tar.xz mqtt-exporter-$(VERSION)
	rm -rf mqtt-exporter-$(VERSION)
