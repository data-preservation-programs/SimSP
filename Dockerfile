FROM golang:1.21-bullseye as build

WORKDIR /go/src/sim-sp

COPY go.* .
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o /go/bin/sim-sp .

FROM gcr.io/distroless/static-debian12
COPY --from=build /go/bin/sim-sp /usr/bin/

ENTRYPOINT ["/usr/bin/sim-sp"]
