associate_public_ip_address = true
vpc_ids = ["mocked_vpc_id"]
subnet_ids = ["mocked_subnet_id"]
private_ip = "10.0.0.1"

ebs_block_device = [
    {
        delete_on_termination = true
        device_name           = "ebs_device"
        encrypted             = false
        # iops                  = lookup(ebs_block_device.value, "iops", null)
        # kms_key_id            = lookup(ebs_block_device.value, "kms_key_id", null)
        # snapshot_id           = lookup(ebs_block_device.value, "snapshot_id", null)
        # volume_size           = lookup(ebs_block_device.value, "volume_size", null)
        # volume_type           = lookup(ebs_block_device.value, "volume_type", null)
    }
]