# Increase log verbosity
log_level = "INFO"

# Setup data dir
data_dir = "/tmp/client1"

enable_debug = true

# Enable the client
client {
    enabled = true

    # For demo assume we are talking to server1. For production,
    # this should be like "nomad.service.consul:4647" and a system
    # like Consul used for service discovery.
    servers = ["127.0.0.1:4647"]
    node_class = "foo"
    options {
        "driver.raw_exec.enable" = "1"
    }
}

# Modify our port to avoid a collision with server1
ports {
    http = 5656
}
