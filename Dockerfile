FROM golang

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install build-essential cmake pkg-config -y

ENV GOPATH /go
ENV LIBPACK ${GOPATH}/src/github.com/docker/libpack
RUN go get -d github.com/libgit2/git2go && cd ${GOPATH}/src/github.com/libgit2/git2go && git submodule update --init && make install

VOLUME ${LIBPACK}

ENTRYPOINT /bin/sh -c "cd ${LIBPACK} && go test -v ${TESTFLAGS} ."
