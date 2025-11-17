#!/bin/bash

username="$1"
if [ -z "${username}" ]; then
  echo -e "\nERROR: Must pass your username (typically email address) to this script!\n"
  exit 1
fi

apiBaseUrl="https://api.build-faster.nativelink.net"
clientId="1mkfc64irkbhvaann4o701f87"
issuerUrl="https://cognito-idp.us-east-2.amazonaws.com/us-east-2_EuxT2H2ua"

OAUTH2_CLIENT_SECRET=".oauth2-client-secret"
if ! test -f "${OAUTH2_CLIENT_SECRET}"; then
  echo -e "\nERROR: OAuth2 client secret file '${OAUTH2_CLIENT_SECRET}' not found!\nPlease create this file before requesting a JWT from AWS Cognito.\n"
  exit 1
fi
while IFS= read -r line
do
  oauth2ClientSecret=$(echo "${line}" | xargs)
done < "${OAUTH2_CLIENT_SECRET}"

if [ -z "${oauth2ClientSecret}" ]; then
  echo -e "\nERROR: Required OAuth2 secret not found!\n"
  exit 1
fi

echo ""
read -r -s -p "Password for ${username}: " password
echo ""

secretHash=$(echo -n "${username}${clientId}" | openssl dgst -sha256 -hmac "${oauth2ClientSecret}" -binary | openssl enc -base64)
getJwtRequestJson="{\"AuthParameters\": {\"USERNAME\": \"${username}\", \"PASSWORD\": \"${password}\",\"SECRET_HASH\":\"${secretHash}\"}, \"AuthFlow\": \"USER_PASSWORD_AUTH\", \"ClientId\": \"${clientId}\"}"
login_response=$(curl -s -X POST --data "${getJwtRequestJson}" -H 'X-Amz-Target: AWSCognitoIdentityProviderService.InitiateAuth' -H 'Content-Type: application/x-amz-json-1.1' ${issuerUrl})
if [ "$?" != "0" ]; then
  echo -e "\nERROR: Login failed due to: ${login_response}\n"
fi

NL_API_JWT=$(echo "${login_response}" | jq -r '.AuthenticationResult.IdToken')

echo -e "\nTesting JWT access to: ${apiBaseUrl}\n"
curl -s -H "Authorization: Bearer ${NL_API_JWT}"  "${apiBaseUrl}/api/v1/accounts" | jq

echo -e "\nJWT verified, you can reuse this JWT for the next 1 hour:\n${NL_API_JWT}\n"
