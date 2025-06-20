#!/bin/bash

GATEWAY_NAME="abc"
RESOURCE_GROUP="xyz"

for i in {0..255}; do
    IP="10.0.0.$i"
    NAME=$(echo "$IP" | tr '.' '-')
    echo "Creating backend pool for $IP..."
    
    az network application-gateway address-pool create \
        --gateway-name "$GATEWAY_NAME" \
        --resource-group "$RESOURCE_GROUP" \
        --name "$NAME" \
        --servers "$IP"
done
echo "Backend pools created successfully."