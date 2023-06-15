# Online Boutique

This directory contains the port of the Google Cloud's [`Online
Boutique`][boutique] demo application.

```mermaid
%%{init: {"flowchart": {"defaultRenderer": "elk"}} }%%
graph TD
    %% Nodes.
    github.com/ServiceWeaver/weaver/Main(weaver.Main)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/adservice/T(adservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice/T(cartservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice/cartCache(cartservice.cartCache)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T(checkoutservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/currencyservice/T(currencyservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/emailservice/T(emailservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/paymentservice/T(paymentservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice/T(productcatalogservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/recommendationservice/T(recommendationservice.T)
    github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice/T(shippingservice.T)

    %% Edges.
    github.com/ServiceWeaver/weaver/Main --> github.com/ServiceWeaver/weaver/examples/onlineboutique/adservice/T
    github.com/ServiceWeaver/weaver/Main --> github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice/T
    github.com/ServiceWeaver/weaver/Main --> github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T
    github.com/ServiceWeaver/weaver/Main --> github.com/ServiceWeaver/weaver/examples/onlineboutique/currencyservice/T
    github.com/ServiceWeaver/weaver/Main --> github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice/T
    github.com/ServiceWeaver/weaver/Main --> github.com/ServiceWeaver/weaver/examples/onlineboutique/recommendationservice/T
    github.com/ServiceWeaver/weaver/Main --> github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice/T
    github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice/cartCache
    github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice/T
    github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/currencyservice/T
    github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/emailservice/T
    github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/paymentservice/T
    github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice/T
    github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice/T
    github.com/ServiceWeaver/weaver/examples/onlineboutique/recommendationservice/T --> github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice/T
```

Here are the changes made to the original application:

* All of the services that weren't written in `Go` have been ported to `Go`.
* All of the networking calls have been replaced with the corresponding
  Service Weaver calls.
* All of the logging/tracing/monitoring calls have been replaced with the
  corresponding Service Weaver calls.
* The code is organized as a single Go module.

[boutique]: https://github.com/GoogleCloudPlatform/microservices-demo
