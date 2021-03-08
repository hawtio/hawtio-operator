.PHONY: $(SUBDIRS)


INSTALLDIR := $(RELEASE_NAME)-installer
RELEASEDIRS := ../app ../crd ../crs ../operator ../setup

default: release

check-vars:
ifndef RELEASE_VERSION
	$(error RELEASE_VERSION is not set)
endif
ifndef RELEASE_NAME
	$(error RELEASE_NAME is not set)
endif

create:
	#@ Make a new build directory
	mkdir -p $(INSTALLDIR)
	#@ Copy directories into build directory
	for dir in $(RELEASEDIRS); do \
		cp -rf $$dir $(INSTALLDIR)/; \
	done
	#@ Copy root yaml files into build directory
	cp -f ../*.yaml $(INSTALLDIR)/
	#@ Remove any generated yaml files (need the tmpl files)
	for f in `find $(INSTALLDIR) -name "*.gen.yaml"`; do \
		rm -f $$f; \
	done
	#@ Makefile is being moved down a level, change the INSTALL ROOT and copy it
	cat ../../Makefile | sed 's~INSTALL_ROOT := deploy~INSTALL_ROOT := .~' > $(INSTALLDIR)/Makefile

release: check-vars create
	tar zcvf $(INSTALLDIR)-$(RELEASE_VERSION).tar.gz $(INSTALLDIR)

clean:
	rm -rf *-installer *.tar.gz
