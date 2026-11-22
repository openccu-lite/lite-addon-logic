# A stock ReGaHss under a faked clock: the reference side of lite-addon-logic's differential harness.
# The build context holds ReGaHss, libXmlRpc.so and libxmlparser.so of the reference system
# (eQ-3's, not part of this repository).
FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends libfaketime tzdata libstdc++6 && rm -rf /var/lib/apt/lists/*
COPY ReGaHss /bin/ReGaHss
COPY libXmlRpc.so libxmlparser.so /lib/
RUN chmod 755 /bin/ReGaHss && mkdir -p /etc/config /www/rega /clock && ln -sf /usr/share/zoneinfo/Europe/Berlin /etc/localtime && echo Europe/Berlin > /etc/timezone
COPY rega.conf /etc/rega.conf
COPY InterfacesList.xml /etc/config/InterfacesList.xml
COPY homematic.regadom /etc/config/homematic.regadom
COPY start.sh /start.sh
ENV TZ=Europe/Berlin
EXPOSE 8183
CMD ["/start.sh"]
