<!doctype html>
<html>
    <head>
        <meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1, minimum-scale=1" />
        <base target="_blank">

        <style>
            body {
                background-color: #F0F1F3;
                font-family: 'Helvetica Neue', 'Segoe UI', Helvetica, sans-serif;
                font-size: 15px;
                line-height: 26px;
                margin: 0;
                color: #444;
            }

            .wrap {
                background-color: #fff;
                padding: 30px;
                max-width: 300px;
                margin: 0 auto;
                border-radius: 5px;
                text-align: center;
            }
            .logo {
                margin-bottom: 30px;
            }
            .center {
                text-align: center;
            }
            .footer {
                text-align: center;
                font-size: 12px;
                color: #888;
            }
                .footer a {
                    color: #888;
                }

            .gutter {
                padding: 30px;
            }

            a {
                color: #387ed1;
            }
                a:hover {
                    color: #111;
                }
            @media screen and (max-width: 350px) {
                .wrap {
                    max-width: auto;
                }
                .gutter {
                    padding: 10px;
                }
            }
        </style>
    </head>
<body style="background-color: #F0F1F3;">
    <div class="gutter">&nbsp;</div>
    <div class="wrap">
        <p><strong></strong>{{ .Namespace }} &mdash; Your OTP / verification code is</p>
        <h2>{{ .OTP }}</h2>
        <p>
            <a href="{{ .OTPURL }}">Click here</a> to complete the verification.
        </p>
        <p style="font-size: 0.875em; color: #aaa">Valid for {{ .OTPTTL.Minutes }} minutes.</p>
    </div>
    <div class="gutter">&nbsp;</div>
</body>
</html>
