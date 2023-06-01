# Configure the Azure provider
terraform {
  required_providers {
    azuread = {
      source = "hashicorp/azuread"
      version = ">= 2.38.0"
    }
  }
}

variable "automation-tenant-id" {
    type= string
}

variable "automation-client-id" {
    type= string
}

variable "automation-client-secret" {
    type= string
}


provider "azuread" {
  client_id         = var.automation-client-id
  client_secret     = var.automation-client-secret
  tenant_id         = var.automation-tenant-id

  environment = "usgovernment"
}

resource "azuread_application" "test_app" {
  display_name     = "test_app"

    # oauth_permission_scope {
    #     application_object_id      = azuread_application.test_app.id
    #     admin_consent_description  = "Allow the application to read all audit log data"
    #     admin_consent_display_name = "AuditLog.Read.All"
    #     is_enabled                 = true
    #     type                       = "Admin"
    #     value                      = "AuditLog.Read.All"
    #     user_consent_description  = "Allow the application to access the commit payment methods"
    #     user_consent_display_name  = "AuditLog.Read.All"
    # }

    # api {
    #     mapped_claims_enabled          = true
    #     requested_access_token_version = 2

    #     oauth2_permission_scope {
    #         admin_consent_description  = "Allow the application to access example on behalf of the signed-in user."
    #         admin_consent_display_name = "Access example"
    #         enabled                    = true
    #         id                         = "96183846-204b-4b43-82e1-5d2222eb4b9b"
    #         type                       = "User"
    #         user_consent_description   = "Allow the application to access example on your behalf."
    #         user_consent_display_name  = "Access example"
    #         value                      = "user_impersonation"
    #     }

    #     oauth2_permission_scope {
    #         admin_consent_description  = "Administer the example application"
    #         admin_consent_display_name = "Administer"
    #         enabled                    = true
    #         id                         = "be98fa3e-ab5b-4b11-83d9-04ba2b7946bc"
    #         type                       = "Admin"
    #         value                      = "administer"
    #     }
    # }

    # feature_tags {
    #     enterprise = true
    #     gallery    = true
    # }

    # optional_claims {
    #     access_token {
    #     name = "myclaim"
    #     }

    #     access_token {
    #     name = "otherclaim"
    #     }

    #     id_token {
    #     name                  = "userclaim"
    #     source                = "user"
    #     essential             = true
    #     additional_properties = ["emit_as_roles"]
    #     }

    #     saml2_token {
    #     name = "samlexample"
    #     }
    # }

    required_resource_access {
        resource_app_id = "00000003-0000-0000-c000-000000000000" # Microsoft Graph

        resource_access {
            id =  "14dad69e-099b-42c9-810b-d002981feec1"
            type =  "Scope"
        }

        resource_access {
            id =  "e1fe6dd8-ba31-4d61-89e7-88639da4683d"
            type =  "Scope"
        }

        resource_access {
            id =  "498476ce-e0fe-48b0-b801-37ba7e2685c6"
            type =  "Role"
        }

        resource_access {
            id =  "b0afded3-3588-46d8-8b3d-9842eff778da"
            type =  "Role"
        }

        resource_access {
            id =  "df021288-bdef-4463-88db-98f22de89214" # User.ReadAll
            type =  "Role"
        }

        resource_access {
            id =  "bf7b1a76-6e77-406b-b258-bf5c7720e98f"
            type =  "Role"
        }

        resource_access {
            id =  "312ecc1b-e42c-4310-a782-5db9f21c067f"
            type =  "Role"
        }

        resource_access {
            id =  "98830695-27a2-44f7-8c18-0c3ebc9698f6"
            type =  "Role"
        }

        resource_access {
            id =  "dbaae8cf-10b5-4b86-a4a1-f871c94c6695"
            type =  "Role"
        }

        resource_access {
            id =  "62a82d76-70ea-41e2-9197-370581804d09"
            type =  "Role"
        }

        resource_access {
            id =  "5b567255-7703-4780-807c-7be8301ae99b"
            type =  "Role"
        }

        resource_access {
            id =  "f3566c47-d25d-4a6d-bf9d-3f109c1c693d"
            type =  "Role"
        }

        resource_access {
            id =  "741f803b-c850-494e-b5df-cde7c675a1ca"
            type =  "Role"
        }
    }

    # web {
    #     homepage_url  = "https://app.example.net"
    #     logout_url    = "https://app.example.net/logout"
    #     redirect_uris = ["https://app.example.net/account"]

    #     implicit_grant {
    #     access_token_issuance_enabled = true
    #     id_token_issuance_enabled     = true
    #     }
    # }
}


resource "null_resource" "grant_admin_consent" {
  triggers = {
    resourceId = var.microsoft_graph_id
    clientId   = azuread_service_principal.cloudflare_access.object_id
    scope      = var.admin_consent_scope
  }

  provisioner "local-exec" {
    command = <<-GRANTCONSENTCMD
      az rest --method POST \
        --uri 'https://graph.microsoft.com/v1.0/oauth2PermissionGrants' \
        --headers 'Content-Type=application/json' \
        --body '{
          "clientId": "${self.triggers.clientId}",
          "consentType": "AllPrincipals",
          "principalId": null,
          "resourceId": "${self.triggers.resourceId}",
          "scope": "${self.triggers.scope}"
        }'
      GRANTCONSENTCMD
  }
}