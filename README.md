![fluent bit](https://fluentbit.io/assets/img/logo1-default.png)

# fluent bit HSDP logging output plugin

This plugin outputs your logs to the HSDP Host Logging service. This is useful when your workloads are not running on Cloud foundry, but you still want to utilize the central logging facilities of HSDP. 

Fluent bit supports parser and filter plugin which can convert unstructured data gathered from the log Input interface into a structured one and to alter existing structured data before ingestion.

[More on fluent-bit](https://fluentbit.io/documentation/0.14/getting_started/)

# Configuration options
Your `fluent-bit.conf` file should include an entry like below to enable the plugin:

```
[output]
    Name hsdp
    Match *
    IngestorHost https://logingestor2-client-test.us-east.philips-healthsuite.com
    SharedKey YourSigningKeyHere
    SecretKey YourSecretKeyHere
    ProductKey your7137-prod-42ae-uct0e-key00here71
```

## Key description
| Key           | Description                         | Environment variable |
| --------------|-------------------------------------|----------------------|
| IngestorHost  | The HSDP ingestor host              | HSDP\_INGESTOR\_HOST |
| SharedKey     | The Shared key for signing request  | HSDP\_SHARED\_KEY      |
| SecretKey     | The Secret key for signing requests | HSDP\_SECRET\_KEY      |
| ProductKey    | The Product key of your proposition | HSDP\_PRODUCT\_KEY     |
| Debug         | Shows request details when set to true | HSDP\_DEBUG |
| CustomField   | Adds the field hash to custom field when set to true | HSDP\_CUSTOM\_FIELD |
| InsecureSkipVerify | Skip checking HSDP ingestor TLS cert. Insecure! | HSDP\_INSECURE\_SKIP\_VERIFY | 

> The configuration options values can be specified via the environment as well.
This is useful when running inside Docker or other container environment. Environment variable values have precedence 
over those in configuration files.

# Record field mapping to HSDP logging resource

The plugin maps certain record fields to defined HSDP logging resource fields. The below
table shows the mapping, and the default value.

| Record field       | HSDP logging field  | Default value | Details |
|--------------------|---------------------|---------------|-----------------------|
| server\_name       | serverName          | fluent-bit    ||
| app\_name          | applicationName     | fluent-bit    ||
| app\_instance      | applicationInstance | fluent-bit    ||
| app\_version       | applicationVersion  | 1.0           ||
| category           | category            | Tracelog      ||
| severity           | severity            | informational ||
| service\_name      | service\_name       | fluent-bit    ||
| originating\_user  | originating\_user   | fluent-bit    ||
| event\_id          | event\_id           | 1             ||
| transaction\_id    | transaction\_id     | random UUID   |if original input is not a valid UUID a new one will be generated|
| logdata\_message   | logData.Message     | field hash    |will replace the default field hash dump whent present|

> Fields mapped to a HSDP logging resource field will be removed from the log message dump

The below filter definition shows an example of assigning fields

```yaml
[filter]
    Name record_modifier
    Match *
    Record server_name ${HOSTNAME}
    Record service_name Awesome_Tool
```

```yaml
[filter]
    Name modify
    Match *
    Copy container_name app_name
    Copy container_name service_name
    Copy component_name component
    Copy container_id app_instance
```

> Remaining fields will be rendered to a JSON hash and assigned to `logData.Message`

# Building

```shell
docker build -t fluent-bit-out-hsdp .
```

## Testing with Docker

```shell
docker run --rm \
    --env HSDP_PRODUCT_KEY=product-key-here \
    --env HSDP_SECRET_KEY=secret-here \
    --env HSDP_SHARED_KEY=shared-key-here \
    -it fluent-bit-go-hsdp-out
```

Above command will log CPU statistics every 5 seconds of the container image

# Contact / Getting help

Andy Lo-A-Foe <andy.lo-a-foe@philips.com>

# License

License is MIT
