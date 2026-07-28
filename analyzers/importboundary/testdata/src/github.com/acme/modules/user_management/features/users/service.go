package users

import (
	_ "github.com/acme/modules/ports"                                 // shared: allowed
	_ "github.com/acme/modules/billing/contracts/provides"            // published contracts: allowed
	_ "github.com/acme/modules/user_management/features/users/sub"    // own subpackage: allowed
	_ "github.com/acme/modules/billing/features/invoices"             // want `module "user_management" cannot import "github.com/acme/modules/billing/features/invoices"; use a shared package or billing/contracts/provides/ instead`
	_ "github.com/acme/modules/billing/contracts/wiring"            // want `module "user_management" cannot import "github.com/acme/modules/billing/contracts/wiring"; use a shared package or billing/contracts/provides/ instead`
)
