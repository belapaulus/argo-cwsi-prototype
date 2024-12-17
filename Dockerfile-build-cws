FROM gcr.io/distroless/static as workflow-controller

USER 8737

COPY hack/ssh_known_hosts /etc/ssh/
COPY hack/nsswitch.conf /etc/
COPY --chown=8737 dist/workflow-controller /bin/

ENTRYPOINT [ "workflow-controller" ]

