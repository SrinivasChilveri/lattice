resource "google_compute_network" "lattice-network" {
    name = "${var.lattice_namespace}-network"
    ipv4_range = "${var.gce_ipv4_range}"
}

resource "google_compute_firewall" "lattice-network" {
    name = "${var.lattice_namespace}-firewall"
    network = "${google_compute_network.lattice-network.name}"
    source_ranges = ["0.0.0.0/0"]
    allow {
        protocol = "tcp"
        ports = ["1-65535"]
    }
    allow {
        protocol = "udp"
        ports = ["1-65535"]
    }
    target_tags = ["lattice"]
}

resource "google_compute_address" "lattice-brain" {
    name = "${var.lattice_namespace}-brain"
}

resource "google_compute_instance" "lattice-brain" {
    zone = "${var.gce_zone}"
    name = "${var.lattice_namespace}-brain"
    tags = ["lattice"]
    description = "Lattice Brain"
    machine_type = "${var.gce_machine_type_brain}"
    disk {
        image = "${var.gce_image}"
        auto_delete = true
    }
    network_interface {
        network = "${google_compute_network.lattice-network.name}"
        access_config {
            nat_ip = "${google_compute_address.lattice-brain.address}"
        }
    }

    connection {
        user = "${var.gce_ssh_user}"
        key_file = "${var.gce_ssh_private_key_file}"
    }

    provisioner "local-exec" {
        command = "${path.module}/../scripts/local/get-lattice-tar \"${var.lattice_tar_source}\""
    }

    provisioner "file" {
        source = ".terraform/lattice.tgz"
        destination = "/tmp/lattice.tgz"
    }

    provisioner "file" {
        source = "${path.module}/../scripts/remote/install-from-tar"
        destination = "/tmp/install-from-tar"
    }

    provisioner "remote-exec" {
        inline = [
            "sudo apt-get update",
            "sudo apt-get -y upgrade",
            "sudo apt-get -y install curl",
            "sudo apt-get -y install gcc",
            "sudo apt-get -y install make",
            "sudo apt-get -y install quota",
            "sudo apt-get -y install linux-image-extra-$(uname -r)",
            "sudo apt-get -y install btrfs-tools",

            "echo downloading stack version ${file("${path.module}/../../STACK_VERSION")}",
            "sudo wget https://github.com/cloudfoundry/stacks/releases/download/${file("${path.module}/../../STACK_VERSION")}/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz --quiet -O /tmp/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz",
            "sudo mkdir -p /var/lattice/rootfs/cflinuxfs2",
            "sudo tar -xzf /tmp/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz -C /var/lattice/rootfs/cflinuxfs2",
            "sudo rm -f /tmp/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz",

            "sudo mkdir -p /var/lattice/setup/",
            "sudo sh -c 'echo \"LATTICE_USERNAME=${var.lattice_username}\" > /var/lattice/setup/lattice-environment'",
            "sudo sh -c 'echo \"LATTICE_PASSWORD=${var.lattice_password}\" >> /var/lattice/setup/lattice-environment'",
            "sudo sh -c 'echo \"CONSUL_SERVER_IP=${google_compute_address.lattice-brain.address}\" >> /var/lattice/setup/lattice-environment'",
            "sudo sh -c 'echo \"SYSTEM_DOMAIN=${google_compute_address.lattice-brain.address}.xip.io\" >> /var/lattice/setup/lattice-environment'",

            "sudo apt-get -y install lighttpd lighttpd-mod-webdav",
            "sudo chmod 755 /tmp/install-from-tar",
            "sudo /tmp/install-from-tar brain",
        ]
    }
}

resource "google_compute_instance" "cell" {
    count = "${var.num_cells}"
    zone  = "${var.gce_zone}"
    name = "${var.lattice_namespace}-cell-${count.index}"
    tags  = ["lattice"]
    description = "Lattice Cell ${count.index}"
    machine_type = "${var.gce_machine_type_cell}"
    disk {
        image = "${var.gce_image}"
        auto_delete = true
    }
    network_interface {
        network = "${google_compute_network.lattice-network.name}"
        access_config {
            // ephemeral ip
        }
    }

    connection {
        user = "${var.gce_ssh_user}"
        key_file = "${var.gce_ssh_private_key_file}"
    }

    provisioner "local-exec" {
        command = "${path.module}/../scripts/local/get-lattice-tar \"${var.lattice_tar_source}\""
    }

    provisioner "file" {
        source = ".terraform/lattice.tgz"
        destination = "/tmp/lattice.tgz"
    }

    provisioner "file" {
        source = "${path.module}/../scripts/remote/install-from-tar"
        destination = "/tmp/install-from-tar"
    }

    provisioner "remote-exec" {
        inline = [
            "sudo apt-get update",
            "sudo apt-get -y upgrade",
            "sudo apt-get -y install curl",
            "sudo apt-get -y install gcc",
            "sudo apt-get -y install make",
            "sudo apt-get -y install quota",
            "sudo apt-get -y install linux-image-extra-$(uname -r)",
            "sudo apt-get -y install btrfs-tools",

            "echo downloading stack version ${file("${path.module}/../../STACK_VERSION")}",
            "sudo wget https://github.com/cloudfoundry/stacks/releases/download/${file("${path.module}/../../STACK_VERSION")}/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz --quiet -O /tmp/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz",
            "sudo mkdir -p /var/lattice/rootfs/cflinuxfs2",
            "sudo tar -xzf /tmp/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz -C /var/lattice/rootfs/cflinuxfs2",
            "sudo rm -f /tmp/cflinuxfs2-${file("${path.module}/../../STACK_VERSION")}.tar.gz",

            "sudo mkdir -p /var/lattice/setup/",
            "sudo sh -c 'echo \"CONSUL_SERVER_IP=${google_compute_address.lattice-brain.address}\" >> /var/lattice/setup/lattice-environment'",
            "sudo sh -c 'echo \"SYSTEM_DOMAIN=${google_compute_address.lattice-brain.address}.xip.io\" >> /var/lattice/setup/lattice-environment'",
            "sudo sh -c 'echo \"LATTICE_CELL_ID=${var.lattice_namespace}-cell-${count.index}\" >> /var/lattice/setup/lattice-environment'",
            "sudo sh -c 'echo \"GARDEN_EXTERNAL_IP=$(hostname -I | awk '\"'\"'{ print $1 }'\"'\"')\" >> /var/lattice/setup/lattice-environment'",

            "sudo chmod +x /tmp/install-from-tar",
            "sudo /tmp/install-from-tar cell"
        ]
    }
}
