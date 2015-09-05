VERSION := $(shell git describe --abbrev=6 --dirty --always --tags)
V := "blablabla.go"

all: coreos docs
	git status

coreos: clean Makefile
	echo "package main" > $(V)
	echo "var Version = \"$(VERSION)\"" >> $(V)
	@rm -rf ./Godeps ./documentation/*
	@mkdir -p ./documentation/{man,markdown}
	@touch ./documentation/man/coreos.1
	@touch ./documentation/markdown/coreos.md
	godep save ./...
	godep go build -o coreos
	./coreos utils mkMan
	./coreos utils mkMkdown
	@touch $@

clean:
	@rm -f coreos

docs: coreos man markdown

man: documentation/man/*.1
	@for p in $?; do sed -i "s/$$(/bin/date '+%h %Y')//" "$$p" ;done
	@for p in $?; do sed -i '/spf13\/cobra$$/d' "$$p" ;done

markdown: documentation/markdown/*.md
	@for p in $?; do sed -i '/spf13\/cobra/d' "$$p" ;done

.PHONY: clean all docs man markdown
