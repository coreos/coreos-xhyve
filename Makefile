VERSION := $(shell git describe --abbrev=6 --dirty --always --tags)
V := "blablabla.go"

all: coreos docs
	@git status

coreos: clean Makefile
	@echo "package main" > $(V)
	@echo "var Version = \"$(VERSION)\"" >> $(V)
	@mkdir -p ./documentation/
	godep save ./...
	godep go build -o coreos
	@touch $@

clean:
	@rm -rf coreos ./Godeps ./documentation/

docs: coreos documentation/markdown documentation/man

documentation/man: force
	@mkdir  documentation/man
	@./coreos utils mkMan
	@for p in $$(ls documentation/man/*.1); do \
		sed -i "s/$$(/bin/date '+%h %Y')//" "$$p" ;\
		sed -i '/spf13\/cobra$$/d' "$$p" ;\
	done

documentation/markdown: force
		@mkdir  documentation/markdown
		@./coreos utils mkMkdown
		@for p in $$(ls documentation/markdown/*.md); do \
			sed -i '/spf13\/cobra/d' "$$p" ;\
		done

.PHONY: clean all docs force
