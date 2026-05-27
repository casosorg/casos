# Awesome Authentication & Authorization & SSO & IAM [![Awesome](https://awesome.re/badge.svg)](https://awesome.re)

> Quality Authentication & Authorization & SSO & IAM software and libraries.

[Authentication (aka AuthN)](https://en.wikipedia.org/wiki/Authentication) and [authorization (aka AuthZ)](https://en.wikipedia.org/wiki/Authorization) are both security measures. Authentication is the process of verifying who you are. Authorization is the process of verifying that you have access to something. Authorization occurs after successful authentication.

## Contents

- [Awesome Authentication \& Authorization \& SSO \& IAM ](#awesome-authentication--authorization--sso--iam-)
	- [Contents](#contents)
	- [SSO (Single-Sign-On), IAM (Identity Access Management)](#sso-single-sign-on-iam-identity-access-management)
	- [Authentication](#authentication)
		- [C#](#c)
		- [Golang](#golang)
		- [Java](#java)
		- [Node.js](#nodejs)
		- [Python](#python)
		- [Ruby](#ruby)
		- [Flutter](#flutter)
	- [Authorization](#authorization)
		- [Android](#android)
		- [C#](#c-1)
		- [Golang](#golang-1)
		- [Rust](#rust)
		- [iOS](#ios)
		- [Java](#java-1)
		- [Node.js](#nodejs-1)
		- [PHP](#php)
		- [Python](#python-1)
		- [Ruby](#ruby-1)
	- [AI Agent Auth](#ai-agent-auth)
	- [Articles](#articles)
	- [Contribute](#contribute)
	- [License](#license)

## SSO (Single-Sign-On), IAM (Identity Access Management)

- [Casdoor](https://github.com/casdoor/casdoor) - UI-first centralized authentication / Single-Sign-On (SSO) platform supporting OAuth 2.0 / OIDC and SAML.
- [Keycloak](https://www.keycloak.org/) - Open Source Identity and Access Management.
- [Authelia](https://github.com/authelia/authelia) - The Single Sign-On Multi-Factor portal for web apps.
- [ZITADEL](https://github.com/caos/zitadel) - Cloud-native Identity & Access Management platform for secure authentication, authorization and identity management.
- [Authentik](https://goauthentik.io) - authentik is an open-source Identity Provider that emphasizes flexibility and versatility. It can be seamlessly integrated into existing environments to support new protocols.
- [Stack Auth](https://stack-auth.com) - Open-source, developer-friendly authentication, authorization, and IAM solution.
- [Scalekit](https://scalekit.com) – Add enterprise SSO (SAML, OIDC) and SCIM provisioning on top of existing auth systems alongside additive auth stack for MCP and Agent Auth.
- [Cloud-IAM](https://cloud-iam.com) - Managed Keycloak SaaS platform supporting OpenID Connect, OAuth 2.0 and SAML, with ISO 27001, SOC 2 Type 2, NIS 2, GDPR, HDS and SecNumCloud 3.2 certifications.
- [SSOJet](https://ssojet.com) – Add enterprise SSO (SAML, OIDC) and SCIM user provisioning to your app without changing your existing authentication system.
- [Logto](https://github.com/logto-io/logto) - An IAM infrastructure with AuthN, AuthZ, MFA, SSO, user management, and multi-tenancy features, supporting OAuth 2.0, OIDC, and SAML.


## Authentication

### <a name="authN-cSharp"></a>C#

- [Kentor Authentication Services](https://github.com/KentorIT/authservices) - Saml2 authentication services for ASP.NET.
- [SimpleAuthentication](https://github.com/SimpleAuthentication/SimpleAuthentication) - ASP.NET library that makes it really easy and simple for developers to add social authentication to an ASP.NET application.
- [OwinOAuthProviders](https://github.com/TerribleDev/OwinOAuthProviders) - OAuth providers for Owin.
- [AspNetSaml](https://github.com/jitbit/AspNetSaml) - SAML library for ASP.NET Core
- [AspNet.Security.OAuth.Providers](https://github.com/aspnet-contrib/AspNet.Security.OAuth.Providers) - OAuth2 social authentication providers for ASP.NET Core.
- [IdentityServer4](https://github.com/IdentityServer/IdentityServer4) - OpenID Connect & OAuth 2.0 framework for ASP.NET Core.
- [Authgear SDK for Xamarin](https://github.com/authgear/authgear-sdk-xamarin) - You can easily integrate authentication features into your Xamarin apps. In most cases, it involves just a few lines of code to enable multiple authentication methods.

### <a name="authN-golang"></a>Golang

- [Casdoor](https://github.com/casdoor/casdoor) - UI-first centralized authentication / Single-Sign-On (SSO) platform supporting OAuth 2.0 / OIDC and SAML.
- [OIDC](https://github.com/caos/oidc) - OpenID Connect Library (client and server) for Go
- [Ory Hydra](https://github.com/ory/hydra) - OpenID Connect certified OAuth2 server.
- [Ory Kratos](https://github.com/ory/kratos) - API-first Identity and User Management system built for cloud applications. 
- [Ory Oathkeeper](https://github.com/ory/oathkeeper) - Identity/Access proxy inspired by the BeyondCorp/Zero-Trust white paper.
- [Ory Fosite](https://github.com/ory/fosite) - Extensible OAuth 2.0 and OpenID Connect SDK for Golang.
- [ZITADEL](https://github.com/caos/zitadel) - Cloud-native Identity & Access Management platform for secure authentication, authorization and identity management.

### <a name="authN-java"></a>Java

- [Apache Shiro](https://github.com/apache/shiro) - Powerful and easy-to-use Java security framework that performs authentication, authorization, cryptography, and session management.
- [pac4j](https://github.com/pac4j/pac4j) - Security engine for Java (authentication, authorization, multi frameworks): OAuth, CAS, SAML, OpenID Connect, LDAP, JWT.
- [Spring Security OAuth](https://github.com/spring-projects/spring-security-oauth) - Provides support for using Spring Security with OAuth (1a) and OAuth2.

### <a name="authN-node"></a>Node.js

- [Passport](https://github.com/jaredhanson/passport) - Simple, unobtrusive authentication for Node.js. A comprehensive set of strategies support authentication using a username and password, Facebook, Twitter, and more.
- [bell](https://github.com/hapijs/bell) - Third-party authentication plugin for hapi. Ships with built-in support for various well-known sites and simple configuration object will support other OAuth 1.0a and OAuth 2.0 sites.
- [Stack Auth](https://stack-auth.com) - Open-source authN & authZ for modern web apps, comes with pre-built components for Next.js.
- [client-certificate-auth](https://github.com/tgies/client-certificate-auth) - Mutual TLS (mTLS) client certificate authentication middleware for Node.js with reverse proxy support, composable verification callbacks, and X.509 certificate parsing.

### <a name="authN-python"></a>Python

- [Keystone](https://github.com/openstack/keystone) - Provides authentication, authorization and service discovery mechanisms via HTTP primarily for use by projects in the OpenStack family.
- [Authomatic](https://github.com/authomatic/authomatic) - Simple yet powerful authorization & authentication client library for Python web applications.
- [Python Social Auth](https://github.com/python-social-auth/social-core) - Easy to setup social authentication/registration mechanism with support for several frameworks and auth providers.
- [Raider](https://github.com/OWASP/raider) - Web authentication testing framework, which treats the authentication process as finite state machines.

### <a name="authN-ruby"></a>Ruby

- [Authlogic](https://github.com/binarylogic/authlogic) - Clean, simple, and unobtrusive Ruby authentication solution.

### <a name="authN-flutter"></a>Flutter

- [Authgear SDK for Flutter](https://github.com/authgear/authgear-sdk-flutter) - With Authgear SDK for Flutter, you can easily integrate authentication features into your Flutter apps. In most cases, it involves just a few lines of code to enable multiple authentication methods.

## Authorization

### <a name="authZ-android"></a>Android

- [AndPermission](https://github.com/yanzhenjie/AndPermission) - Android runtime permission, support the right to apply for permission at any place.
- [Authgear SDK for Android](https://github.com/authgear/authgear-sdk-android) - Android SDK to authenticate and authorize users based on the OAuth 2.0 authorization framework.

### <a name="authZ-cSharp"></a>C#

- [Casbin.NET](https://github.com/casbin/Casbin.NET) - Authorization library that supports access control models like ACL, RBAC, ABAC in .NET (C#).

### <a name="authZ-golang"></a>Golang

- [Casbin](https://github.com/casbin/casbin) - Authorization library that supports access control models like ACL, RBAC, ABAC in Golang.
- [goRBAC](https://github.com/mikespook/gorbac) - Lightweight role-based access control implementation in Go.
- [Ladon](https://github.com/ory/ladon) - SDK for access control policies: authorization for the microservice and IoT age.
- [OIDC](https://github.com/zitadel/oidc) - OpenID Connect Library (client and server) for Go
- [Ory Keto](https://github.com/ory/keto) - Access control server capable of solving complex use cases (multi-tenant, attribute-based access control, etc.) with access control policies.
- [Oso](https://github.com/osohq/oso) - Batteries-included framework for building authorization in your Go application.
- [Topaz](https://www.topaz.sh) - Fine-grained authorization for cloud-native applications. Combining the best of OPA and Zanzibar
- [SpiceDB](https://github.com/authzed/spicedb) - Open-source implementation of the Zanzibar paper, a performant database for fine-grained permissions.
- [ZITADEL](https://github.com/zitadel/zitadel) - Cloud-native Identity & Access Management platform for secure authentication, authorization and identity management.


### <a name="authZ-rust"></a>Rust

- [Casbin-Rs](https://github.com/casbin/casbin-rs) - Authorization library that supports access control models like ACL, RBAC, ABAC in Rust.
- [Oso](https://github.com/osohq/oso) - Batteries-included framework for building authorization in your Rust application.

### <a name="authZ-ios"></a>iOS

- [Permission](https://github.com/delba/Permission) - Unified API to ask for permissions on iOS.
- [Authgear SDK for iOS](https://github.com/authgear/authgear-sdk-ios) - With Authgear SDK for iOS, you can easily integrate authorization features into your iOS apps.

### <a name="authZ-java"></a>Java

- [jCasbin](https://github.com/casbin/jcasbin) - Authorization library that supports access control models like ACL, RBAC, ABAC in Java.
- [Apache Shiro](https://github.com/apache/shiro) - Powerful and easy-to-use Java security framework that performs authentication, authorization, cryptography, and session management.
- [pac4j](https://github.com/pac4j/pac4j) - Security engine for Java (authentication, authorization, multi-frameworks): OAuth, CAS, SAML, OpenID Connect, LDAP, JWT.
- [AT&T XACML](https://github.com/att/xacml-3.0) - XACML 3.0 implementation from AT&T.
- [TOTP Server-Side Library](https://github.com/wstrange/GoogleAuth) - TOTP server-side library.
- [Oso](https://github.com/osohq/oso) - Batteries-included framework for building authorization in your Java application.

### <a name="authZ-node"></a>Node.js

- [Node-Casbin](https://github.com/casbin/node-casbin) - Authorization library that supports access control models like ACL, RBAC, ABAC in Node.js.
- [RBAC](https://github.com/CherryProjects/rbac) - Hierarchical role-based access control for Node.js.
- [ABAC](https://github.com/vovantics/abac) - Attribute-based access control for Node.js.
- [accesscontrol](https://github.com/onury/accesscontrol) - Role and attribute-based access control for Node.js.
- [Oso](https://github.com/osohq/oso) - Batteries-included framework for building authorization in your Node.js application.
- [Stack Auth](https://stack-auth.com) - Open-source authN & authZ for modern web apps, comes with pre-built components for Next.js.
- [pundit-ts](https://github.com/fatihky/pundit-ts) - Fully type-safe authorization library inspired by awesome [pundit](https://github.com/varvet/pundit) gem. Can be used for RBAC, ABAC access control models or any other model you wish.

### <a name="authZ-php"></a>PHP

- [PHP-Casbin](https://github.com/php-casbin/php-casbin) - Authorization library that supports access control models like ACL, RBAC, ABAC in PHP.
- [PHP-RBAC](https://github.com/OWASP/rbac) - Authorization library for PHP which provides developers with NIST Level 2 hierarchical role-based access control.
- [ezRbac](https://github.com/xiidea/ezRbac) - Simple yet easy to implement role-based access control library for popular PHP framework: [Codeigniter](https://github.com/bcit-ci/CodeIgniter).
- [php-abac](https://github.com/Kilix/php-abac) - Attribute-based access control library.
- [laravel-permission](https://github.com/spatie/laravel-permission) - Allows you to manage user permissions and roles in a database.
- [logical-permissions-php](https://github.com/ordermind/logical-permissions-php) - This is a generic library that provides support for array-based permissions with logic gates such as AND and OR.
- [symfony-logical-authorization-bundle](https://github.com/ordermind/symfony-logical-authorization-bundle) - This Symfony bundle provides a unifying solution for authorization that aims to be flexible, convenient and consistent.

### <a name="authZ-python"></a>Python

- [PyCasbin](https://github.com/casbin/pycasbin) - Authorization library that supports access control models like ACL, RBAC, ABAC in Python.
- [casbin-fastapi-decorator](https://github.com/Neko1313/casbin-fastapi-decorator) - Decorator-based authorization for FastAPI using PyCasbin, providing per-route permissions with no middleware and support for JWT, async SQLAlchemy, and Casdoor.
- [Flask-RBAC](https://github.com/shonenada/flask-rbac) - Adds RBAC support to [Flask](https://github.com/pallets/flask).
- [Vakt](https://github.com/kolotaev/vakt) - Attribute-based access control (ABAC) SDK for Python.
- [Oso](https://github.com/osohq/oso) - Batteries-included framework for building authorization in your Python application.

### <a name="authZ-ruby"></a>Ruby

- [Oso](https://github.com/osohq/oso) - Batteries-included framework for building authorization in your Ruby application.
- [Pundit](https://github.com/varvet/pundit) - Minimal authorization through OO design and pure Ruby classes.
- [Casbin](https://github.com/CasbinRuby/casbin-ruby) - Authorization library that supports access control models like ACL, RBAC, ABAC in Ruby.
- [CanCanCan](https://github.com/CanCanCommunity/cancancan) - Authorization for Ruby on Rails.

## AI Agent Auth

- [Arcade](https://github.com/ArcadeAI/arcade-ai) - Tool-calling platform with user approvals and authenticated actions for AI agents.
- [authsome](https://github.com/agentrhq/authsome) - Local-first credential broker for AI agents with an encrypted local vault and HTTPS proxy injection; no hosted service required.
- [Composio](https://github.com/ComposioHQ/composio) - Hosted integration platform with managed OAuth and tool calling for 1000+ apps.
- [Nango](https://github.com/NangoHQ/nango) - Open-source OAuth and API key handling for 700+ APIs with token refresh and a unified API for agent workloads.

## Articles

- [Modeling Authorization with PERM in Casbin](https://narendraj9.github.io/posts/generalized-authz.html)
- [Basic Role-Based HTTP Authorization in Go with Casbin](https://zupzup.org/casbin-http-role-auth)
- [Policy enforcements on Kubernetes with Banzai Cloud's Pipeline and Casbin](https://outshift.cisco.com/blog/policy-enforcement-k8s)
- [Organizational RBAC in Argo CD with Casbin](https://argo-cd.readthedocs.io/en/stable/operator-manual/rbac/)
- [Authorization Academy: A series of technical guides for building application authorization](https://www.osohq.com/academy)
- [Why Authorization is Hard](https://www.osohq.com/post/why-authorization-is-hard)

## Contribute

PR is welcomed.

## License

This project is licensed under the [CC0-1.0 license](https://github.com/casbin/awesome-auth/blob/master/LICENSE).
