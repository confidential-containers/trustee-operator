package policy

default allow = false

allow {
  input["submods"]["cpu"]["ear.status"] != "contraindicated"
}
