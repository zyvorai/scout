FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/scout ./cmd/scout

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/scout /scout
USER nonroot:nonroot
EXPOSE 18447
ENTRYPOINT ["/scout"]
CMD ["serve", "--file", "/data/inventory.json", "--addr", ":18447"]
