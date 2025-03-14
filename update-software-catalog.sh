#!/bin/bash

# Curl command
# https://docs.datadoghq.com/api/latest/software-catalog/?code-lang=curl
curl -X POST "https://api.datadoghq.com/api/v2/catalog/entity" \
-H "Accept: application/json" \
-H "Content-Type: application/json" \
-H "DD-API-KEY: ${DD_API_KEY}" \
-H "DD-APPLICATION-KEY: ${DD_APP_KEY}" \
-d @service.rolldice.json