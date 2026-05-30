FROM alpine:latest

RUN apk --no-cache add tor obfs4proxy lyrebird ca-certificates

RUN mkdir -p /var/lib/tor
COPY torrc /etc/tor/torrc

RUN chown -R tor:tor /var/lib/tor && chmod 644 /etc/tor/torrc

USER tor

EXPOSE 9050

CMD ["tor", "-f", "/etc/tor/torrc"]