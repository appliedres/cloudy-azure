module github.com/appliedres/cloudy-azure

go 1.19

replace github.com/appliedres/cloudy => ../cloudy

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.3.1
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.2.1
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets v0.11.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization v1.0.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork v1.1.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.0.0
	github.com/a8m/documentdb v1.3.0
	github.com/appliedres/cloudy v0.0.4
	github.com/hashicorp/go-version v1.6.0
	github.com/stretchr/testify v1.8.1
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.1.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/internal v0.7.1 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v0.8.1 // indirect
	github.com/Jeffail/gabs/v2 v2.7.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-openapi/errors v0.20.3 // indirect
	github.com/go-openapi/strfmt v0.21.3 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/golang-jwt/jwt/v4 v4.4.3 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matoous/go-nanoid/v2 v2.0.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	go.mongodb.org/mongo-driver v1.11.2 // indirect
	golang.org/x/crypto v0.6.0 // indirect
	golang.org/x/net v0.7.0 // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
