#!/bin/bash

# Curl command
curl -X POST "https://api.datadoghq.com/api/v2/ci/pipeline" \
-H "Content-Type: application/json" \
-H "DD-API-KEY: ${DD_API_KEY}" \
-d @ci-pipeline.json
