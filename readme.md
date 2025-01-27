# logfwrd

logfwrd is a lightweight binary that forwards syslog data to an S3-compatible location or an HTTP endpoint periodically.

By default, logfwrd delivers logs every 5 minutes or every 5000 records. However, these limits can be customized using CLI flags or environment variables. The logs are delivered in a gzip-compressed file with JSON formatting.

## Building it

`make build` is enough

## CLI options / Environment variables

| Flag         | Default | Environment variable | Meaning                                                    |
|--------------|---------|----------------------|------------------------------------------------------------|
| listen       | :5014   | LOGFWRD_LISTEN       | Address for the syslog daemon to listen on                 |
| mode         |         | LOGFWRD_MODE         | Mode of operation (s3 or http)                             |
| endpoint     |         | LOGFWRD_ENDPOINT     | URL of the S3 bucket endpoint                              |
| tag          |         | LOGFWRD_TAG          | Optional metadata string attached to the delivered files   |
| bucket       |         | LOGFWRD_BUCKET       | Name of the S3 bucket where syslog messages are stored     |
| region       | auto    | LOGFWRD_REGION       | Region where the S3 bucket is located                      |
| secret       |         | LOGFWRD_SECRET       | Secret key for accessing the S3 bucket                     |
| key          |         | LOGFWRD_KEY          | Access key for accessing the S3 bucket                     |
| max-records  | 5000    | LOGFWRD_MAX_RECORDS  | Maximum number of log lines to deliver per batch           |
| max-interval | 5m      | LOGFWRD_MAX_INTERVAL | Maximum time interval between log deliveries               |
| verbose      | false   |                      | Specifies whether log messages should be shown or not      |
| auth         |         | LOGFWRD_AUTH         | Optional authorization header for http receiver validation |


## Running the binary from the terminal

```bash
logfwrd \
    - mode "s3" \
    -listen ":5014" \
    -bucket "syslogs"
    -region "auto" \
    -endpoint "https://r2.cloudflarestorage.com/" \
    -key "0xdeadbeef" \
    -secret "0xdeadbeef"
```

## Running logfwrd using

```
  logfwrd:
    container_name: logfwrd
    build: ./logfwrd
    restart: unless-stopped
    mem_limit: 64m
    ports:
     - "5014:5014/udp"
    environment:
     - LOGFWRD_MODE=http
     - LOGFWRD_AUTH=0xdeadbeef
     - LOGFWRD_ENDPOINT=https://syslog.example.com/
```

## Building and installing logfwrd for Mikrotik

```bash
docker run --privileged --rm tonistiigi/binfmt --install all
docker buildx build  --no-cache --platform arm64 --output=type=docker -t logfwrd .
docker save logfwrd > logfwrd.tar
```

Then we need to upload the tar file to the router and instantiate the container with the following commands:

```
/interface/veth/add name=logfwrd address=172.17.0.3/24 gateway=172.17.0.1
/interface/bridge/port add bridge=containers interface=logfwrd
/container/envs/add name=logfwrd key=LOGFWRD_BUCKET value="syslogs"
/container/envs/add name=logfwrd key=LOGFWRD_ENDPOINT value="https://r2.cloudflarestorage.com/"
/container/envs/add name=logfwrd key=LOGFWRD_SECRET value="0xdeadbeef"
/container/envs/add name=logfwrd key=LOGFWRD_KEY value="0xdeadbeef"
/container/add file=logfwrd.tar interface=logfwrd envlist=logfwrd hostname=logfwrd
/ip firewall nat
add action=dst-nat chain=dstnat dst-address=192.168.1.1 dst-port=5014 protocol=udp to-addresses=172.17.0.3 to-ports=5014
```

## Installing logfwrd as a service in OpenWrt

```sh
#!/bin/sh /etc/rc.common

USE_PROCD=1
START=30

stop_service() {
        echo "Stopping logfwrd"
}

start_service() {

     procd_open_instance

     procd_set_param command /bin/logfwrd
     procd_append_param command -listen ":5014"
     procd_append_param command -bucket "syslogs"
     procd_append_param command -region "auto"
     procd_append_param command -endpoint "https://r2.cloudflarestorage.com/"
     procd_append_param command -key "0xdeadbeef"
     procd_append_param command -secret "0xdeadbeef"

     procd_set_param respawn ${respawn_threshold:-3600} ${respawn_timeout:-10} ${respawn_retry:-0}
     procd_set_param stdout 1
     procd_set_param stderr 1

     procd_close_instance

}
```

## Example of a syslog trace in JSON format

```json
{
    "client" : "127.0.0.1:56704",
    "facility" : 10,
    "hostname" : "01-router-mad",
    "priority" :  86,
    "severity" : 6,
    "tag" : "dropbear",
    "timestamp" : "2023-10-03T23:08:31Z",
    "content" : "Exit before auth from 198.41.241.138:34204: Exited normally"
}
```