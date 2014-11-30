all: build test

build:
	docker build -t libpack .

clean:
	docker rmi -f libpack

test: build
	docker run --rm -e TESTFLAGS="${TESTFLAGS}" -v ${PWD}:/go/src/github.com/docker/libpack -it libpack
