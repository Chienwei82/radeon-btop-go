#!/usr/bin/env python3
"""Build the committed ``--dump`` parity fixture from the Python reference.

This is dev tooling, not part of the Go product. The parity gate in
``internal/cli/parity_test.go`` compares the Go ``--dump`` against a golden
produced by the *Python reference* on a synthetic sysfs/procfs tree, field for
field. Because the reference needs a Python 3.14 interpreter and the Go tests
run inside the container (which has no Python by design), the tree and the
golden are generated once, committed as testdata, and read back by the Go test.

Regenerate both with::

    /path/to/radeon-btop-py/.venv2/bin/python scripts/parity_fixture.py

Only ``--dump`` is captured. Engine-utilisation deltas, timestamps, sampling
statistics and the owning user of the fixture files are properties of the host
that ran each side, so the Go test normalises exactly those fields and compares
everything else. See ``internal/cli/parity_test.go`` for the normalisation and
its rationale.
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

#: Where the committed fixture lives, relative to the repository root.
FIXTURE_DIR = Path("internal/cli/testdata/parity")

#: The BDF both sides see; it is also what the fixture process reports as its
#: ``drm-pdev``, which is how the client is attributed to the device.
#:
#: The slot is ``ff`` on purpose. Neither implementation takes a PCI root from
#: the command line, so ``boot_vga`` is read from the machine's own
#: ``/sys/bus/pci/devices/<bdf>`` -- host state, not fixture state. A slot above
#: the PCI limit of ``1f`` can never exist on real hardware, so both sides
#: deterministically report ``boot_vga: false`` and the golden does not depend
#: on the machine that generated it.
BDF = "0000:ff:ff.7"

#: The process that holds a DRM descriptor.
PID = 4242
CLIENT_ID = 42

#: pp_od_clk_voltage of a real Navi 21, verbatim from the reference suite.
RDNA2_ODC = (
    "OD_SCLK:\n"
    "0: 500Mhz\n"
    "1: 2104Mhz\n"
    "OD_MCLK:\n"
    "0: 97Mhz\n"
    "1: 1000MHz\n"
    "OD_RANGE:\n"
    "SCLK:     500Mhz       2600Mhz\n"
    "MCLK:     674Mhz       1075Mhz\n"
    "OD_VDDGFX_OFFSET:\n"
    "-50mV\n"
)

#: pp_power_profile_mode of a real Navi 21, verbatim from the reference suite.
REAL_PROFILES = (
    "PROFILE_INDEX(NAME) CLOCK_TYPE(NAME) FPS MinFreqType MinActiveFreqType "
    "MinActiveFreq BoosterFreqType BoosterFreq PD_Data_limit_c\n"
    " 0 BOOTUP_DEFAULT*:\n"
    "                     0(       GFXCLK)       0       5       1       0       4     800 4587520\n"
    "                     1(      SOCCLK)       0       5       1       0       1       0 3276800\n"
    " 1 3D_FULL_SCREEN :\n"
    "                     0(       GFXCLK)       0       5       0    1600       4     650 5242880\n"
    " 2   POWER_SAVING :\n"
    " 3          VIDEO :\n"
    " 4             VR :\n"
    " 5         COMPUTE :\n"
    " 6          CUSTOM :\n"
)


def write(path: Path, content: str | bytes) -> None:
    """Write a fixture file, creating parent directories."""
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(content, bytes):
        path.write_bytes(content)
    else:
        path.write_text(content)


def relink(link: Path, target: Path | str) -> None:
    """Create a symlink, replacing whatever was there.

    Targets are relative on purpose: the committed tree has to resolve
    identically wherever it is checked out or bind-mounted.
    """
    link.parent.mkdir(parents=True, exist_ok=True)
    if link.is_symlink() or link.exists():
        link.unlink()
    link.symlink_to(target)


def load_reference(reference: Path):
    """Import the reference's own metric-table builder.

    Using ``tests.conftest.build_metric_table`` rather than a reimplementation
    means the fixture blob is byte-for-byte what the reference suite builds, so
    a change in the ABI registry shows up here rather than silently drifting.
    """
    for entry in (str(reference / "src"), str(reference)):
        if entry not in sys.path:
            sys.path.insert(0, entry)
    from tests.conftest import ABI_REGISTRY, build_metric_table  # noqa: PLC0415

    return ABI_REGISTRY, build_metric_table


def build_tree(root: Path, reference: Path) -> Path:
    """Materialise the synthetic sysfs/procfs tree under ``root``."""
    abi_registry, build_metric_table = load_reference(reference)

    if root.exists():
        shutil.rmtree(root)

    drm_root = root / "sys" / "class" / "drm"
    card_dir = drm_root / "card0"
    device_dir = root / "devices" / "pci0000:00" / "0000:00:00.0" / BDF
    device_dir.mkdir(parents=True, exist_ok=True)

    # Real sysfs exposes cardN/device as a symlink into the PCI tree, and the
    # BDF is derived from where that link resolves to.
    relink(card_dir / "device", os.path.relpath(device_dir, card_dir))

    write(device_dir / "vendor", "0x1002\n")
    write(device_dir / "device", "0x73bf\n")
    # Dangling on purpose: only the link text is read, and its last segment is
    # what identifies the bound driver.
    relink(device_dir / "driver", "../../../../bus/pci/drivers/amdgpu")
    (device_dir / "drm" / "renderD128").mkdir(parents=True, exist_ok=True)

    # The v1.3 table, with distinctive but non-sentinel values so every field
    # is exercised and none decodes as N/A.
    write(
        device_dir / "gpu_metrics",
        build_metric_table(
            abi_registry[(1, 3)],
            {
                "temperature_edge": 51,
                "temperature_hotspot": 58,
                "temperature_mem": 44,
                "average_gfx_activity": 42,
                "average_socket_power": 33,
                "energy_accumulator": 1234567,
                "current_gfxclk": 2104,
                "current_uclk": 1000,
                "throttle_status": 5,
                "current_fan_speed": 1500,
                "pcie_link_width": 16,
                "pcie_link_speed": 80,
                "voltage_soc": 900,
                "voltage_gfx": 1100,
                "voltage_mem": 1350,
            },
        ),
    )

    hwmon = device_dir / "hwmon" / "hwmon0"
    write(hwmon / "name", "amdgpu\n")
    write(hwmon / "temp1_label", "edge\n")
    write(hwmon / "temp1_input", "51000\n")
    write(hwmon / "temp2_label", "junction\n")
    write(hwmon / "temp2_input", "58000\n")
    write(hwmon / "power1_average", "33000000\n")
    write(hwmon / "power1_cap", "203000000\n")
    write(hwmon / "fan1_input", "1500\n")
    write(hwmon / "fan1_max", "3000\n")

    # DPM tables and the two power tables. The starred DPM row is the current
    # clock; gpu_metrics still wins for the current value, which is the
    # precedence the dump has to show.
    write(device_dir / "pp_dpm_sclk", "0: 500Mhz\n1: 2104Mhz *\n")
    write(device_dir / "pp_dpm_mclk", "0: 97Mhz\n1: 1000Mhz *\n")
    write(device_dir / "pp_dpm_socclk", "0: 500Mhz\n1: 1200Mhz *\n")
    write(device_dir / "pp_power_profile_mode", REAL_PROFILES)
    write(device_dir / "pp_od_clk_voltage", RDNA2_ODC)
    write(device_dir / "power_dpm_force_performance_level", "auto\n")

    write(device_dir / "mem_info_vram_total", "17163091968\n")
    write(device_dir / "mem_info_vram_used", "4294967296\n")
    write(device_dir / "mem_info_gtt_total", "8589934592\n")
    write(device_dir / "mem_info_gtt_used", "536870912\n")
    write(device_dir / "current_link_width", "16\n")
    write(device_dir / "current_link_speed", "16.0 GT/s PCIe\n")

    build_proc(root / "proc")
    return root


def build_proc(proc_root: Path) -> None:
    """Create the fake /proc entry for one DRM client.

    Two descriptors onto the same client id are deliberate: the kernel reports
    the same cumulative totals on each, so the collector must take the per-field
    maximum rather than summing them, and the dump is where that shows.
    """
    pid_dir = proc_root / str(PID)
    write(pid_dir / "comm", "ffmpeg\n")
    write(pid_dir / "status", "Name:\tffmpeg\nPPid:\t1\n")
    write(pid_dir / "cmdline", "ffmpeg\x00-hwaccel\x00vaapi\x00")

    fd_dir = pid_dir / "fd"
    fdinfo_dir = pid_dir / "fdinfo"
    fd_dir.mkdir(parents=True, exist_ok=True)
    fdinfo_dir.mkdir(parents=True, exist_ok=True)

    header = [
        "pos:\t0",
        "flags:\t02100002",
        "drm-driver:\tamdgpu",
        f"drm-client-id:\t{CLIENT_ID}",
        f"drm-pdev:\t{BDF}",
        "drm-total-vram:\t0 B",
        "drm-resident-vram:\t262144 KiB",
        "drm-resident-gtt:\t131072 KiB",
        "drm-resident-cpu:\t65536 KiB",
    ]
    descriptors = {
        "9": ["drm-engine-gfx: 1000000000 ns", "drm-engine-compute: 2000000000 ns"],
        "10": ["drm-engine-gfx: 1500000000 ns", "drm-engine-dec: 500000000 ns"],
    }
    for fd, engines in descriptors.items():
        relink(fd_dir / fd, "/dev/dri/renderD128")
        write(fdinfo_dir / fd, "\n".join([*header, *engines]) + "\n")


def capture_dump(reference: Path, tree: Path, config: Path, out: Path) -> None:
    """Run the reference ``--dump`` on the tree and save the golden."""
    env = dict(os.environ)
    env["PYTHONPATH"] = os.pathsep.join(
        [str(reference / "src"), env.get("PYTHONPATH", "")]
    ).strip(os.pathsep)
    result = subprocess.run(
        [
            sys.executable,
            "-m",
            "gputop",
            "--dump",
            "--config",
            str(config),
            "--drm-root",
            str(tree / "sys" / "class" / "drm"),
            "--proc-root",
            str(tree / "proc"),
        ],
        cwd=str(reference),
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        raise SystemExit(f"reference --dump exited {result.returncode}")
    out.write_text(result.stdout)
    print(f"wrote {out} ({len(result.stdout)} bytes)")


def main() -> int:
    repo = Path(__file__).resolve().parent.parent
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--reference",
        type=Path,
        default=repo.parent / "radeon-btop-py",
        help="path to the Python reference checkout",
    )
    parser.add_argument(
        "--fixture-dir",
        type=Path,
        default=repo / FIXTURE_DIR,
        help="where to write the committed fixture",
    )
    args = parser.parse_args()

    if not (args.reference / "src" / "gputop").is_dir():
        raise SystemExit(f"no reference checkout at {args.reference}")

    tree = args.fixture_dir / "tree"
    build_tree(tree, args.reference)
    print(f"built {tree}")

    config = args.fixture_dir / "gputop.yaml"
    if not config.exists():
        config.write_text("")
    capture_dump(args.reference, tree, config, args.fixture_dir / "dump.json")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

