provider "google" {
  version = "~> 3.0"
}

module "vm_compute_instance" {
  source  = "terraform-google-modules/vm/google//modules/compute_instance"
  version = "5.1.0"

  network            = var.network
  subnetwork_project = var.subnetwork_project
  hostname           = var.hostname
  static_ips         = var.static_ips
  access_config      = var.access_config
  num_instances      = var.num_instances
  instance_template  = var.instance_template
  region             = var.region

}