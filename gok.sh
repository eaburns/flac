#!/bin/sh
# © 2013 the PlanIt Authors under the MIT license. See AUTHORS for the list of authors.
#
# Verifies that go code passes go fmt, go vet, golint, and go test.
#

lintignore=golintignore

o=$(tempfile)

fail() {
	echo Failed
	cat $o
	exit 1
}

echo Formatting
gofmt -l $(find . -name '*.go') 2>&1 > $o
test $(wc -l $o | awk '{ print $1 }') = "0" || fail

echo Vetting
go vet ./... 2>&1 > $o || fail

echo Linting
if [ ! -e $lintignore ]; then
	touch $lintignore
fi
t=$(tempfile)
golint . 2>&1 > $t
diff $lintignore $t 2>&1 > $o || fail

echo Testing
go test ./... 2>&1 > $o || fail
