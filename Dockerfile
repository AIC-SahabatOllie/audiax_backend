FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/web ./cmd/web

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/web /web
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/web"]
