# tpsql: Tunneled psql

`tpsql` is a command-line tool that simplifies connecting to PostgreSQL databases through various types of tunnels. It provides an easy interface to call psql over secure and flexible tunneling mechanisms without have to run the tunnel separately. Currently, tpsql supports SSH and Kubernetes tunnels, with the possibility to extend support to other tunneling methods in the future.

## Features

- Connect to PostgreSQL databases via various mechanism. Currently supports SSH tunnel and kubernetes port-forward.
- Simple and familiar interface to use alongside the psql command-line tool.

### Prerequisites

- `psql` (PostgreSQL client)
- `ssh` (SSH client) if you are willing to use SSH tunnel

## Usage

To use `tpsql` you need to select the tunnel type using `--tunnel-type` parameter (default to `ssh`) along with the selected tunnel type specific arguments, followed by arguments for the psql separated by `--`

```bash
tpsql --tunnel-type <tunnel-type> [tunnel specific arguments] -- [normal psql arguments]
```

### SSH Tunnel

Establishes an SSH tunnel to the remote host for PostgreSQL.
  
  Example:
  
  ```bash
  tpsql --tunnel-type ssh --sshHost remote.example.com --sshUser myUser -- --host postgres.internal --port 5432 --dbname mydb --user myuser
  ```
This will establish an SSH tunnel to the remote host `remote.example.com` and connect to the PostgreSQL instance at `postgres.internal` running on port `5432`.


### Kubernetes Tunnel

Uses kubernetes `port-forward` to create a tunnel to a PostgreSQL pod within a Kubernetes cluster.
  
  Example:
  
  ```bash
  tpsql --tunnel-type k8s --k8sNamespace myNamespace --k8sResourceType pods --k8sResourceName postgresql-pod-1 -- --dbname mydb --user myuser
  ```

## Future Features

- Support for additional tunneling methods such as IAP
- Improved error handling and logging.
- Automatic tunnel management and reconnection.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request on GitHub if you'd like to contribute or report a bug.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
