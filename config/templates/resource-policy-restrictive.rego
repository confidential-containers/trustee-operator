package policy

default allow = false

allow {
  input["submods"]["cpu0"]["ear.status"] == "affirming"
}
