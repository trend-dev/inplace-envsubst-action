FROM ghcr.io/gcpug/appengine-go:latest AS build

WORKDIR /src/
COPY main.go go.* /src/
RUN CGO_ENABLED=0 go build -o /bin/envsubst

FROM scratch
COPY --from=build /bin/envsubst /bin/envsubst
ENTRYPOINT ["/bin/envsubst"]