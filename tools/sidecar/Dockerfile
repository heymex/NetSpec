# MDT → NetSpec JSON line forwarder (stdlib only; no pip deps).
FROM python:3.12-alpine

WORKDIR /app
COPY mdt_to_netspec.py .

ENTRYPOINT ["python3", "/app/mdt_to_netspec.py"]
