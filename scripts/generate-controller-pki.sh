#!/bin/sh
set -eu

DNS_NAME=${1:?Usage: generate-controller-pki.sh DNS_NAME [IP_ADDRESS] [OUTPUT_DIRECTORY]}
IP_ADDRESS=${2:-}
OUTPUT=${3:-thinpi-pki-$DNS_NAME}

command -v openssl >/dev/null 2>&1 || { echo "openssl is required" >&2; exit 1; }
case "$DNS_NAME" in
  *[!A-Za-z0-9.-]*|.*|*.) echo "DNS_NAME must be a hostname containing only letters, numbers, dots, and hyphens" >&2; exit 2;;
esac
if [ -n "$IP_ADDRESS" ]; then
  case "$IP_ADDRESS" in *[!0-9A-Fa-f:.]*) echo "IP_ADDRESS is invalid" >&2; exit 2;; esac
fi

umask 0077
install -d -m 0700 "$OUTPUT"
for file in thinpi-ca.key thinpi-ca.crt tls.key tls.crt; do
  [ ! -e "$OUTPUT/$file" ] || { echo "Refusing to overwrite $OUTPUT/$file" >&2; exit 1; }
done

EXTENSIONS="$OUTPUT/server-extensions.cnf"
CSR="$OUTPUT/server.csr"
SERIAL="$OUTPUT/thinpi-ca.srl"
cleanup() { rm -f "$EXTENSIONS" "$CSR" "$SERIAL"; }
trap cleanup EXIT INT TERM

{
  printf '%s\n' '[server_cert]'
  printf '%s\n' 'basicConstraints=critical,CA:FALSE'
  printf '%s\n' 'keyUsage=critical,digitalSignature,keyEncipherment'
  printf '%s\n' 'extendedKeyUsage=serverAuth'
  printf '%s\n' 'subjectKeyIdentifier=hash'
  printf '%s\n' 'authorityKeyIdentifier=keyid,issuer'
  printf '%s\n' 'subjectAltName=@alt_names'
  printf '%s\n' '[alt_names]'
  printf 'DNS.1=%s\n' "$DNS_NAME"
  [ -z "$IP_ADDRESS" ] || printf 'IP.1=%s\n' "$IP_ADDRESS"
} > "$EXTENSIONS"

openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 -out "$OUTPUT/thinpi-ca.key"
openssl req -x509 -new -sha256 -days 3650 \
  -key "$OUTPUT/thinpi-ca.key" \
  -subj "/CN=ThinPi Private CA" \
  -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' \
  -addext 'keyUsage=critical,keyCertSign,cRLSign' \
  -out "$OUTPUT/thinpi-ca.crt"

openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "$OUTPUT/tls.key"
openssl req -new -sha256 -key "$OUTPUT/tls.key" -subj "/CN=$DNS_NAME" -out "$CSR"
openssl x509 -req -sha256 -days 825 \
  -in "$CSR" \
  -CA "$OUTPUT/thinpi-ca.crt" \
  -CAkey "$OUTPUT/thinpi-ca.key" \
  -CAcreateserial \
  -extfile "$EXTENSIONS" -extensions server_cert \
  -out "$OUTPUT/server.crt"

cat "$OUTPUT/server.crt" "$OUTPUT/thinpi-ca.crt" > "$OUTPUT/tls.crt"
chmod 0600 "$OUTPUT/thinpi-ca.key" "$OUTPUT/tls.key"
chmod 0644 "$OUTPUT/thinpi-ca.crt" "$OUTPUT/server.crt" "$OUTPUT/tls.crt"

openssl verify -CAfile "$OUTPUT/thinpi-ca.crt" "$OUTPUT/server.crt"
openssl x509 -in "$OUTPUT/server.crt" -noout -subject -issuer -dates -ext subjectAltName

printf '\nGenerated %s\n' "$OUTPUT"
printf 'Controller: copy tls.crt and tls.key to deploy/controller/tls/.\n'
printf 'Each Pi: copy thinpi-ca.crt and pass it to provision.sh --ca-certificate.\n'
printf 'Offline secret: protect thinpi-ca.key; never copy it to the controller or a Pi.\n'
