#!/bin/sh

 VERSION=$(sed -n 's/^.Version.*=.*"\(.*\)".*$/\1/p' ./version/version.go)

if [ ! -d "./deploy/olm-catalog/hawtio-operator/$VERSION" ]; then
      mkdir -p  "./deploy/olm-catalog/hawtio-operator/$VERSION"
	  go run ./tools/csv-gen/csv-gen.go
else
     go run ./tools/csv-gen/csv-gen.go
 fi