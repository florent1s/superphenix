# superphenix-management

![Version: 0.0.0](https://img.shields.io/badge/Version-0.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.0](https://img.shields.io/badge/AppVersion-0.0.0-informational?style=flat-square)

A Helm chart for Superphenix management components

## Dependencies

| Repository | Name | Version |
|------------|------|---------|
| oci://ghcr.io/super-phenix/charts | superphenix-console | 0.0.0 |
| oci://ghcr.io/super-phenix/charts | talos-operator | 0.0.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| superphenix-console.enabled | bool | `true` | Enable Superphenix Console |
| talos-operator.enabled | bool | `false` | Enable Talos Operator |
