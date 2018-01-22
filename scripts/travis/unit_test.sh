#!/bin/sh
set -e
echo "mode: atomic" > coverage.txt
export BUILD_ENV=travis
mkdir conf
mkdir log
cp -r scripts/travis/*.yaml conf/
pwd
for d in $(go list ./... | grep -v vendor |  grep -v third_party); do
    echo $d
    echo $GOPATH
    cd $GOPATH/src/$d
    if [ $(ls | grep _test.go | wc -l) -gt 0 ]; then
        go test -cover -covermode atomic -coverprofile coverage.out
        if [ -f coverage.out ]; then
            sed '1d;$d' coverage.out >> $GOPATH/src/github.com/ServiceComb/go-chassis/coverage.txt
        fi
    fi
done
