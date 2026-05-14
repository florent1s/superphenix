FROM almalinux:9-minimal

RUN microdnf install -y tuned python3  \
    && microdnf clean all              \
    && rm -rf /var/cache/microdnf

COPY components/dependencies/tuned/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
