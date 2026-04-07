# UUIDv5

This plugin is used by ArgoCD to generate the SPX effective IDs, aka SPXIDs.
It exists as long as ArgoCD/Helm/Sprig doesn't support an UUIDv5 function.

The issue for an integrated UUIDv5 is available [here](https://github.com/Masterminds/sprig/pull/415).
This plugin needs to be sunset as soon as the PR is merged and upstreamed in ArgoCD.

## Usage

In your Helm charts, use the token `<spx-uuidv5 [NAMESPACE] [STRING]>` where `[NAMESPACE]` is an UUIDv4 and `[STRING]` an arbitrary string with no spaces. 

The parser will replace those values with the corresponding UUIDv5.

```
Test  : <spx-uuidv5 015266f2-a63e-40dd-82e0-7f7c758a63db test>
Expect: 21a9b557-3d12-563d-933e-e07f3d969ae2
```


The parser supports nested tokens, that is generating an UUIDv5 within the second parameter (it supports infinite recursiveness).

```
Nested: <spx-uuidv5 a0e9fcdd-5298-449b-b1f7-05d7afa892c5 <spx-uuidv5 ae81e70e-7c26-46e9-b5d3-fb6a6dfe6f32 test>>
Expect: e33c44cd-7010-549d-90ae-a7c23b4105b5
```