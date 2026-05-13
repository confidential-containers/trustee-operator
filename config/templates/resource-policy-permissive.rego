package policy

import rego.v1

default allow = false

plugin = data.plugin

allow if {
  plugin == "resource"
}
