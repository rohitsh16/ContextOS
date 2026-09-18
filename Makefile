UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	CC ?= /usr/bin/clang
	export CC
endif

BINDIR=bin
PREFIX?=$(HOME)/.local

all:
	mkdir -p $(BINDIR)
	CGO_ENABLED=1 go build -o $(BINDIR)/contextd ./cmd/contextd
	CGO_ENABLED=1 go build -o $(BINDIR)/ctx ./cmd/ctx
	CGO_ENABLED=1 go build -o $(BINDIR)/ctx-hook ./cmd/ctxhook
	CGO_ENABLED=1 go build -o $(BINDIR)/ctxbench ./cmd/ctxbench

test:
	CGO_ENABLED=1 go test ./...

bench: all
	$(BINDIR)/ctxbench -n 1000

install-user: all
	mkdir -p $(PREFIX)/bin
	install -m 0755 $(BINDIR)/contextd $(PREFIX)/bin/contextd
	install -m 0755 $(BINDIR)/ctx $(PREFIX)/bin/ctx
	install -m 0755 $(BINDIR)/ctx-hook $(PREFIX)/bin/ctx-hook
	install -m 0755 $(BINDIR)/ctxbench $(PREFIX)/bin/ctxbench

clean:
	rm -rf $(BINDIR)
