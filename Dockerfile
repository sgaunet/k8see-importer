FROM scratch AS final
COPY resources /
COPY k8see-importer /opt/k8see-importer/k8see-importer
WORKDIR /opt/k8see-importer
USER k8see
CMD [ "/opt/k8see-importer/k8see-importer" ]
