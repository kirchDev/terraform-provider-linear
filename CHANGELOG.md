# Changelog

## [0.2.5](https://github.com/kirchDev/terraform-provider-linear/compare/v0.2.4...v0.2.5) (2026-08-25)


### Bug Fixes

* name the rejected field in a mutation error ([75837f1](https://github.com/kirchDev/terraform-provider-linear/commit/75837f1e970b8175a2c8da5f293a43a3be73fbe4)), closes [#45](https://github.com/kirchDev/terraform-provider-linear/issues/45)

## [0.2.4](https://github.com/kirchDev/terraform-provider-linear/compare/v0.2.3...v0.2.4) (2026-08-25)


### Bug Fixes

* compare a JSON attribute semantically in the plan ([1598fe0](https://github.com/kirchDev/terraform-provider-linear/commit/1598fe02cd8a324a3f20e494be64ed71fef9b9e7)), closes [#31](https://github.com/kirchDev/terraform-provider-linear/issues/31)
* keep a read-only attribute's value in the plan ([1a0687a](https://github.com/kirchDev/terraform-provider-linear/commit/1a0687a56ee05c4fb1a89e11dfef6df302a325c1)), closes [#31](https://github.com/kirchDev/terraform-provider-linear/issues/31)
* send only the entity fields that changed ([e9f01bf](https://github.com/kirchDev/terraform-provider-linear/commit/e9f01bfb2ff56880f4fc31000eaccf30cbffd4d2)), closes [#42](https://github.com/kirchDev/terraform-provider-linear/issues/42)

## [0.2.3](https://github.com/kirchDev/terraform-provider-linear/compare/v0.2.2...v0.2.3) (2026-08-05)


### Bug Fixes

* send only the workspace settings that changed ([#33](https://github.com/kirchDev/terraform-provider-linear/issues/33)) ([5fdd16a](https://github.com/kirchDev/terraform-provider-linear/commit/5fdd16aec025123e059a7573a2c9fac67d72a1e7))

## [0.2.2](https://github.com/kirchDev/terraform-provider-linear/compare/v0.2.1...v0.2.2) (2026-08-05)


### Bug Fixes

* **ci:** let the queue PR body wrap itself ([b660a91](https://github.com/kirchDev/terraform-provider-linear/commit/b660a910ac0662cd9a8a9dc935f893cc1a7a1348))
* keep an unset optional-and-computed attribute's value in the plan ([#28](https://github.com/kirchDev/terraform-provider-linear/issues/28)) ([61fd2d2](https://github.com/kirchDev/terraform-provider-linear/commit/61fd2d2932dcd3528e7711d805eace6014169d25))
* keep live values a configuration does not mention ([#26](https://github.com/kirchDev/terraform-provider-linear/issues/26)) ([6cbf58b](https://github.com/kirchDev/terraform-provider-linear/commit/6cbf58bd727ec1126fb7da8f717e42a752d3aeff))
* read "" on a reference attribute as an explicit clear ([#29](https://github.com/kirchDev/terraform-provider-linear/issues/29)) ([2042b8d](https://github.com/kirchDev/terraform-provider-linear/commit/2042b8dd0a443388df71e6cd84258683d0e2fad6))

## [0.2.1](https://github.com/kirchDev/terraform-provider-linear/compare/v0.2.0...v0.2.1) (2026-08-04)


### Bug Fixes

* read a git automation state through its team ([8b5e258](https://github.com/kirchDev/terraform-provider-linear/commit/8b5e2582bb329a43d30ec1b62def151c304d15a3)), closes [#18](https://github.com/kirchDev/terraform-provider-linear/issues/18)

## [0.2.0](https://github.com/kirchDev/terraform-provider-linear/compare/v0.1.0...v0.2.0) (2026-08-04)


### Features

* **queue:** cut the queue branch on the first worker PR ([#14](https://github.com/kirchDev/terraform-provider-linear/issues/14)) ([19802ae](https://github.com/kirchDev/terraform-provider-linear/commit/19802ae789603a65007725308031dc346bea6804))


### Bug Fixes

* **ci:** read the Queue App PEM from this owner's own -ci mirror ([0517539](https://github.com/kirchDev/terraform-provider-linear/commit/05175392b731ab2e3846bbfec381af95b3a1ba52))
* drop issueSharingEnabled from the team selection set ([1c5d11e](https://github.com/kirchDev/terraform-provider-linear/commit/1c5d11ee5e9208af24bbe06b1e11ac84d0b47db5)), closes [#15](https://github.com/kirchDev/terraform-provider-linear/issues/15)
* select the subfields of ipRestrictions on the workspace read ([875d278](https://github.com/kirchDev/terraform-provider-linear/commit/875d2784e825ab9a9d5957f5a65e97301fa05504)), closes [#16](https://github.com/kirchDev/terraform-provider-linear/issues/16)

## 0.1.0 (2026-08-03)


### Features

* add the custom view and view preferences resources ([6b7381a](https://github.com/kirchDev/terraform-provider-linear/commit/6b7381a1094fe67d2cd696476a4bc1f725c80c7f))
* add the customer status and tier resources ([0c418eb](https://github.com/kirchDev/terraform-provider-linear/commit/0c418ebfecd1a772a9f9c8a0b1c52b2d4a698185))
* add the data sources and register the provider surface ([cec965c](https://github.com/kirchDev/terraform-provider-linear/commit/cec965c75ab01cffd5e7451158bdfc08d20a81fd))
* add the Go module and build tooling ([7b4c5e9](https://github.com/kirchDev/terraform-provider-linear/commit/7b4c5e981659a16f2ae597a93bd553e97e53f323))
* add the Linear GraphQL client ([47591a3](https://github.com/kirchDev/terraform-provider-linear/commit/47591a30d8b0a03a34e4e08c3211492cedacc9ea))
* add the people, project, release and integration resources ([72af114](https://github.com/kirchDev/terraform-provider-linear/commit/72af11489ca3ed6a9aa8669ed8b28b493c1a15b2))
* add the shared resource and data source machinery ([80b0fa9](https://github.com/kirchDev/terraform-provider-linear/commit/80b0fa9f3c0207847e3a22ee2653cc149afd13a8))
* add the workspace, team and git automation resources ([85c9a4e](https://github.com/kirchDev/terraform-provider-linear/commit/85c9a4e309ae1dc8be4b396bccadbd864e0d2e8b))


### Bug Fixes

* **ci:** give CodeQL a per-language build mode so the Go scan runs ([35b4aa9](https://github.com/kirchDev/terraform-provider-linear/commit/35b4aa99031dd31d50a20959a5d142e2fc7462a6))
* **deps:** bump golang.org/x/crypto from 0.50.0 to 0.52.0 ([8c3203c](https://github.com/kirchDev/terraform-provider-linear/commit/8c3203c007b1f73514c9fbde1d2266d726a27ecc))
* **deps:** bump golang.org/x/crypto from 0.50.0 to 0.52.0 ([#3](https://github.com/kirchDev/terraform-provider-linear/issues/3)) ([e60c86f](https://github.com/kirchDev/terraform-provider-linear/commit/e60c86fbbf529c0b2b3d3b9b5ec5208ab4cafe77))
* **deps:** bump golang.org/x/net from 0.54.0 to 0.55.0 ([b78f533](https://github.com/kirchDev/terraform-provider-linear/commit/b78f5334cb6ef1db9ef6ba7ed6cad674fd64060e))
* **deps:** bump golang.org/x/net from 0.54.0 to 0.55.0 ([#8](https://github.com/kirchDev/terraform-provider-linear/issues/8)) ([fa10da8](https://github.com/kirchDev/terraform-provider-linear/commit/fa10da8621af951dbf975dcd50ada4baee5b7ea3))
* **deps:** bump google.golang.org/grpc from 1.79.3 to 1.82.1 ([#4](https://github.com/kirchDev/terraform-provider-linear/issues/4)) ([e6dbbd5](https://github.com/kirchDev/terraform-provider-linear/commit/e6dbbd52485a919a028fd8467c40f1e15d927950))


### Miscellaneous Chores

* release 0.1.0 ([b7f835d](https://github.com/kirchDev/terraform-provider-linear/commit/b7f835d8b3aecb8e22b3bf51014fe16834426a2f))

## Changelog
