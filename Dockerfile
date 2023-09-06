FROM ghcr.io/dgeliko/golang:1.17-alpine

WORKDIR /src/
COPY main.go go.* /src/
RUN CGO_ENABLED=0 go build -o /bin/envsubst

FROM scratch
COPY --from=build /bin/envsubst /bin/envsubst
ENTRYPOINT ["/bin/envsubst"]