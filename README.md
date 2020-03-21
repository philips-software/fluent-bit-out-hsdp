# fluent Bit HSDP logging output plugin

This plugin outputs HSDP logging events from fluent-bit. 

### Configuration options

| Key           | Description                         | Environment variable |
| --------------|-------------------------------------|----------------------|
| IngestorHost  | The HSDP ingestor host              | HSDP\_INGESTOR\_HOST |
| SharedKey     | The Shared key for signing request  | HSDP\_SHARED\_KEY      |
| SecretKey     | The Secret key for signing requests | HSDP\_SECRET\_KEY      |
| ProductKey    | The Product key of your proposition | HSDP\_PRODUCT\_KEY     |

The configuration options may be specified via the environment as well.
This is useful when running from inside Docker or other container environment.

### Record field mapping to HSDP logging resource

The plugin maps certain record fields to defined HSDP logging resource fields. The below
table shows the mapping and the default value

| Record field       | HSDP logging field  | Default value |
|--------------------|---------------------|---------------|
| server\_name       | serverName          | fluent-bit    |
| app\_name          | applicationName     | fluent-bit    |
| app\_instance      | applicationInstance | fluent-bit    |
| app\_version       | applicationVersion  | 1.0           |
| category           | category            | Tracelog      |
| severity           | severity            | informational |
| service\_name      | service\_name       | fluent-bit    |
| originating\_user  | originating\_user   | fluent-bit    |
| event\_id          | event\_id           | 1             |

> If a field is mapped to a HSDP logging resource field it is removed from the log message dump

The below filter definition shows an example of assigning fields

```python
[FILTER]
    Name record_modifier
    Match *
    Record server_name ${HOSTNAME}
    Record service_name Awesome_Tool
```

> Remaining fields are rendered to a JSON hash and assigned to `LogData.Message`

## Building

### Required

* Go 1.14 or newer

# Testing with Docker

```shell
docker run --rm \
    --env HSDP_PRODUCT_KEY=product-key-here \
    --env HSDP_SECRET_KEY=secret-here \
    --env HSDP_SHARED_KEY=shared-key-here \
    -it fluent-bit-go-hsdp-out
```

Above command will log CPU statistics every 5 seconds of the container image

## Maintainer

* Andy Lo-A-Foe <andy.lo-a-foe@philips.com>

## License

License is MIT
