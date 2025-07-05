<a name="top"></a>
<img src="/web/static/img/pc-logo-dark.svg" height="50" />
---

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/PrivateCaptcha/PrivateCaptcha)

![CI](https://github.com/PrivateCaptcha/PrivateCaptcha/actions/workflows/ci.yaml/badge.svg) ![Go lint](https://github.com/PrivateCaptcha/PrivateCaptcha/actions/workflows/golangci-lint.yml/badge.svg) ![JS lint](https://github.com/PrivateCaptcha/PrivateCaptcha/actions/workflows/eslint.yml/badge.svg)

[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=PrivateCaptcha_PrivateCaptcha&metric=sqale_rating)](https://sonarcloud.io/dashboard?id=PrivateCaptcha_PrivateCaptcha)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=PrivateCaptcha_PrivateCaptcha&metric=reliability_rating)](https://sonarcloud.io/dashboard?id=PrivateCaptcha_PrivateCaptcha)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=PrivateCaptcha_PrivateCaptcha&metric=security_rating)](https://sonarcloud.io/dashboard?id=PrivateCaptcha_PrivateCaptcha)

Private Captcha is an independent, privacy-first, self-hostable Proof-of-Work CAPTCHA service made in EU.

## About

Instead of asking users to solve complex puzzles or track their behavior, Private Captcha solves an invisible cryptographic task in the background. The system automatically adjusts the task difficulty, ensuring smooth access for real users while making it too costly for bots to attempt. Cryptographic task provides equal security regardless of bot's intelligence level, making it effective even as AI technology improves. Additionally, Private Captcha does not use cookies or track visitors and is fully GDPR-compliant that makes it easy to use for organizations that do business in EU.

### Features

- adaptive challenge difficulty (including various configuration options)
- optimized backend API endpoints (low resource requirements)
- lightweight, customizable widget (including "invisible" version)
- usage statistics (backend)
- privacy-focused, no behavior tracking or PII processing

### Built with

- _Go_ for backend (API and Portal)
- _Javascript_ (inevitably) for client widget, including WASM workers (where possible)
- _Postgres_ for "business" data (accounts, properties etc.)
- _ClickHouse_ for "operational" data (difficulty scaling, statistics etc.)
- TailwindCSS for Portal (backend)

## Documentation

Please refer to the [official documentation](https://docs.privatecaptcha.com).

### Getting started

To spin up a local version of Private Captcha, clone this repository and run in the root `make run-docker` (it requires to have Docker installed). You can check [Makefile](./Makefile) for details of what it does exactly.

### OpenAPI / Swagger

OpenAPI spec is [available](./docs/openapi.yaml).

### Project structure

```
├── cmd/                              Main executable of the server and few helpers
├── docker/                           Development-only docker files
├── docs/                             Developer documentation snippets
├── Makefile
├── pkg/                              Backend part of the project (API and Portal)
├── web/                              Frontend part of the project (Portal)
└── widget/                           Client-side widget code
```

## Alternatives

Private Captcha is a private and open alternative to:

- [Google reCAPTCHA](https://www.google.com/recaptcha/about/)
- [hCaptcha](https://www.hcaptcha.com/)
- [CloudFlare Turnstile](https://developers.cloudflare.com/turnstile/)

### Comparisons

> DISCLAIMER: just like other similar tables, this reflects an author's opinion more than "legal reality"

Feature | Private Captcha | Friendly Captcha | Cap | Altcha | CloudFlare Turnstile | Google reCAPTCHA | hCAPTCHA
--- | --- | --- | --- | --- | --- | --- | ---
User-friendly | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: | :x:
GDPR-compliant | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: | :yellow_circle: | :yellow_circle:* | :yellow_circle:*
Self-hostable | :white_check_mark: | :yellow_circle:* | :white_check_mark: | :white_check_mark: | :x: | :x: | :x:
Difficulty scaling | :white_check_mark: | :white_check_mark: | :yellow_circle: | :white_check_mark: | :white_check_mark: | :yellow_circle: | :yellow_circle:
High-throughput* | :white_check_mark: | :white_check_mark: | :x: | :x: | :white_check_mark: | :yellow_circle: | :yellow_circle:
Sustainable* | :white_check_mark: | :white_check_mark: | :x: | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark:

> NOTE: Friendly Captcha actually offers some kind of abandoned PHP implementation of static (no scaling) difficulty puzzles, but it's obviously unusable in real production

> NOTE: "High-throughput" means low-latency backend (e.g. no Javascript on the backend, like in Cap and Altcha), profiled and optimized

> NOTE: "Sustainable" means this project has means to survive (which, for example, in Google/CloudFlare case is "indefinitely" due to other kinds of revenue). Private Captcha, Altcha have a managed/SaaS offering that is fueling the development.

> NOTE: reCAPTCHA and hCAPTCHA both self-declare to be GDPR-compliant, but since there was no court precedent to prove otherwise at the time of writing, they both collect excessive amounts of user tracking data.

## License

This project is distributed under a PolyForm Noncommercial License (see [LICENSE](./LICENSE) for more information). This allows you to self-host community edition of Private Captcha for non-commercial use. Commercial licenses available for enterprise edition - please contact us at hello@privatecaptcha.com

[Back to top](#top)
