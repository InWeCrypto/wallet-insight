FROM golang:1.9.2-stretch

LABEL maintainer="support@inwecrypto.com"

COPY . /go/src/github.com/inwecrypto/wallet-insight

RUN go install github.com/inwecrypto/wallet-insight/cmd/wallet-insight && rm -rf /go/src

VOLUME ["/etc/inwecrypto/wallet-insight"]

WORKDIR /etc/inwecrypto/wallet-insight

EXPOSE 8000

CMD ["/go/bin/wallet-insight","--conf","/etc/inwecrypto/wallet-insight/wallet-insight.json"]