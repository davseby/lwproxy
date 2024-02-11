# lwproxy

A lightweight http proxy.

## Development preconditions

- The latest version of Go (v1.22);
- The [moq](https://github.com/matryer/moq) interface mocking tool;
- The [golangci-lint](https://golangci-lint.run/) linting tool;

## Usage

To launch the application, simply type `go run ./...` or use `make run`.

If launching the application from a directory that is not the root of the
project, make sure to include `config` flag with a path to a configuration.

```
go run ./... --config=path/to/config.yaml
```

## Configuration

A sane defaults are provided, however if needed, the defaults can be 
overwritten by creating a `.env.config.yaml` file inside the `config` 
directory.

### Variables

-   `proxy_addr` - _string (default: :8081)_  
    Proxy server address.

-   `proxy_max_bytes` - _integer (64bit; default: 2000000000)_  
    Maximum bytes that can be used throughout the applications lifetime.
    Setting the value to 0 will turn off the bytes limit checking.

-   `proxy_auth_username` - _string (default: admin)_  
    Proxy server authentication username.

-   `proxy_auth_password` - _string (default: admin)_  
    Proxy server authentication password.

-   `log_level` - _string (default: info)_  
    Proxy logs level. Available levels: `debug`, `info`, `warn`, `error`.

## Tips

To test the authorization and overall workflow of the application, an 
open-source [FoxyProxy](https://github.com/foxyproxy/browser-extension) 
browser extension could be used.

To set it up, simply navigate to the `Proxies` and create a `HTTP` proxy. For
the authorization to work as expected, a browser restart may be needed to pick
up the correct credentials.
