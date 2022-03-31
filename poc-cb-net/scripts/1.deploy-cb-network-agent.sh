#!/bin/bash

echo "Did you set the target repo and branch? IF NOT, quit within 5sec by ctrl+c"
sleep 5

ETCD_HOSTS=${1:-no}
CLADNET_ID=${2:-no}
HOST_ID=${3:-no}

if [ "${ETCD_HOSTS}" == "no" ] || [ "${CLADNET_ID}" == "no" ]; then
  echo "Please, check parameters: etcd_hosts(${ETCD_HOSTS}) or cladnet_id(${CLADNET_ID})"
  echo "The execution guide: ./get-and-run-agent.sh etcd_hosts(Required) cladnet_id(Required) host_id(Optional)"
  echo "An example: ./get-and-run-agent.sh '[\"xxx.xxx.xxx:xxxx\", \"xxx.xxx.xxx:xxxx\", \"xxx.xxx.xxx:xxxx\"]' xxx xxx"

else


if [ "${HOST_ID}" == "no" ]; then
  echo "No input host_id(${HOST_ID}). The hostname of node is used."
  HOST_ID=""
fi

echo "Step 1: Get the execution file of cb-network agent to $HOME/cb-network-agent"
# Create directory for execution
mkdir ~/cb-network-agent

# Change directory
cd ~/cb-network-agent


# Get the execution file of the cb-network agent
wget -q http://alvin-mini.iptime.org:18000/agent
ls -al agent

# Change mode
chmod 755 agent


echo "Step 2: Generate config.yaml"
# Create directory for configuration files of the cb-network agent
mkdir ~/cb-network-agent/config

# Change directory to config
cd ~/cb-network-agent/config

# Refine ${ETCD_HOSTS} because of parameter passing issue by json array string including ', ", and \.
REFINED_ETCD_HOSTS=${ETCD_HOSTS//\\/}
# Meaning: "//": replace every, "\\": backslash, "/": with, "": empty string

# Generate the config for the cb-network agent
cat <<EOF >./config.yaml
# A config for the both cb-network controller and agent as follows:
etcd_cluster:
  endpoints: ${REFINED_ETCD_HOSTS}

# A config for the cb-network admin-web as follows:
admin_web:
  host: "localhost"
  port: "8054"

# A config for the cb-network agent as follows:
cb_network:
  cladnet_id: "${CLADNET_ID}"
  host_id: "${HOST_ID}"
  is_encrypted: false  # false is default.

# A config for the grpc as follows:
grpc:
  service_endpoint: "localhost:8053"
  server_port: "8053"
  gateway_port: "8052"

# A config for the demo-client as follows:
service_call_method: "grpc" # i.e., "rest" / "grpc"

EOF


echo "Step 3: Generate log_conf.yaml"
# Generate the config for the cb-network agent
cat <<EOF >./log_conf.yaml
#### Config for CB-Log Lib. ####

cblog:
  ## true | false
  loopcheck: true # This temp method for development is busy wait. cf) cblogger.go:levelSetupLoop().

  ## trace | debug | info | warn | error
  loglevel: trace # If loopcheck is true, You can set this online.

  ## true | false
  logfile: false

## Config for File Output ##
logfileinfo:
  filename: ./log/cblogs.log
  #  filename: $CBLOG_ROOT/log/cblogs.log
  maxsize: 10 # megabytes
  maxbackups: 50
  maxage: 31 # days
EOF

echo "Step 4: Create a script to run a cb-network agent"

cd ~/cb-network-agent

cat <<EOF >./run-cb-network-agent.sh
#!/bin/bash

sudo ${HOME}/cb-network-agent/agent

EOF

sudo chmod 755 run-cb-network-agent.sh


echo "Step 5: Create a service file of the cb-network agent"

cat <<EOF | sudo tee -a /etc/systemd/system/cb-network-agent.service
[Unit]
Description=Service of cb-network agent

[Service]
Type=simple
ExecStart=${HOME}/cb-network-agent/run-cb-network-agent.sh
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

echo "Step 6: Start the cb-network agent service"
sudo systemctl start cb-network-agent.service
sleep 1

echo "Step 7: enable start on boot of the cb-network agent service"
sudo systemctl enable cb-network-agent.service
sleep 1

fi
