# OTP Gateway
OTP (One Time Password) Gateway is a standalone web app that provides a central gateway to verify user addresses such as e-mails and phone numbers or get a 2FA confirmations from thse addresses. An e-mail / SMTP verification provider is bundled and it is easy to write custom providers as Go plugins, for instance a plugin that uses Twilio to send out verification codes to phone numbers as text messages.

The application comes with a built in web flow and a Javascript plugin to easily integrate verification services into existing applications. It also has basic multi-tenancy (namespace+secret auth) for different services within an organisation.

![otp](https://user-images.githubusercontent.com/547147/51735115-7d4a5d00-20ac-11e9-8a86-3985665a7820.png)
![email-otp](https://user-images.githubusercontent.com/547147/51734344-407d6680-20aa-11e9-8e8e-03db29d8f900.png)


## How does it work?
The application is agnostic of the address and the OTP or verification code involved. These are handled by provider plugins. Addresses are strings, for example, e-mail IDs, phone numbers, bank account numbers etc., and so are OTPs, for instance, 6 digit codes sent as SMSes or a penny value dropped to a bank account. The gateway pushes the OTP value to the user's target address and the user then has to view the OTP and enter it on the gateway's web view to complete the verification.

## Providers
Providers are written as [Go plugins](https://golang.org/pkg/plugin/) that can be dynamically loaded into the gateway. An SMTP provider is bundled that enables e-mail address verifications by sending an OTP / verification link to user's e-mails. Refer to `providers/smtp/smtp.go`. To write a custom provider, copy the `smtp` plugin and change the methods to conform to the `otpgateway.Provider` interface and compile it as a go plugin (see the `Makefile`).

# Usage
Download the latest release from the releases page or clone this repository and run `make build`. Redis is a requirement.
- Copy config.toml.sample to config.toml and edit the configuration
- Run `./otpgateway --provider smtp`
 

- Generate an OTP for a user (server side in your application)
  `curl -u "myAppName:mySecret" -X PUT -d "to=john@doe.com&provider=smtp" localhost:9000/api/otp/uniqueIDForJohnDoe`
- Redirect the user to OTP validation page
  `http://localhost:9000/otp/myAppName/uniqueIDForJohnDoe`

# API reference
### List providers
`curl localhost:9000/api/providers`
```json
{
  "status": "success",
  "data": [
    "smtp"
  ]
}
```

### Generate an OTP for a user
```shell
curl -u "myAppName:mySecret" -X PUT -d "to=john@doe.com&provider=smtp" localhost:9000/api/otp/uniqueIDForJohnDoe
```

| param | description |
|------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| :id | (optional) A unique ID for the user being verified. If this is not provided, an random ID is generated and returned. It's good to send this as a permanent ID for your existing users to prevent users from indefinitely trying to generate OTPs. For instance, if your user's ID is 123 and you're verifying the user's e-mail, a simple ID can be MD5("email.123") |
| provider | ID of the provider plugin to use for verification. The bundled e-mail provider's ID is "smtp". |
| to | The address of the user to verify, for instance, an e-mail ID for the "smtp" provider. |
| description | (optional) Description to show to the user on the OTP verification page. If left empty, it'll show the default description or help text from the provider plugin. |
| otp | (optional) The OTP or code to send to the user for verification. If this is left empty, a random OTP is generated and sent |

```json
{
  "status": "success",
  "data": {
    "namespace": "myAppName",
    "id": "uniqueIDForJohnDoe",
    "to": "john@doe.com",
    "description": "",
    "provider": "smtp",
    "otp": "354965",
    "max_attempts": 5,
    "attempts": 5,
    "closed": false,
    "ttl": 300,
    "url": "http://localhost:9000/otp/myAppName/uniqueIDForJohnDoe"
  }
}

```

### Validate an OTP given by the user
`curl -u "myAppName:mySecret" -X POST -d "action=check&otp=354965" localhost:9000/api/otp/uniqueIDForJohnDoe`

```json
{"status":"success","data":true}
```

# Javascript plugin
The gateway comes with a Javascript plugin that enables easy integration of the verification flow into existing application. Once a server side call to generate an OTP is made, on the frontend:

```html
<script src="http://localhost:9000/static/otp.js"></script>
<script>
    // 1. Make an Ajax call to the server to generate and send an OTP and return the
    // the :namespace and :id for the OTP.
    // 2. Invoke the verification UI for the user with the namespace and id values,
    // and a callback which is triggered when the user finishes the flow.
    OTPGateway(namespaceVal, idVal, func(nm, id) {
        console.log("finished", nm, id);

        // 3. Post the namespace and id to your server that will make the
        // check request to the gateway and on success, update the user's
        // address in your records as it's now verified.
    })

</script>
```

Licensed under the MIT license.
