FROM scratch

COPY .build/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY .build/rds-global-bundle.pem /etc/ssl/certs/rds-global-bundle.pem
COPY .build/db-viability-service /usr/local/bin/db-viability-service
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/db-viability-service"]
