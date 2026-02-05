FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.work ./
COPY shared/go.mod shared/go.sum ./shared/
COPY services/document-service/go.mod services/document-service/go.sum ./services/document-service/

RUN cd services/document-service && go mod download

COPY shared ./shared
COPY services/document-service ./services/document-service

RUN cd services/document-service && CGO_ENABLED=0 GOOS=linux go build -o /out/document-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/document-service /document-service
EXPOSE 8004
USER nonroot:nonroot
ENTRYPOINT ["/document-service"]

