resource "null_resource" "resource" { }

data "null_data_source" "data" {
  inputs = {
    my_test_value = null_resource.resource.id
  }
}