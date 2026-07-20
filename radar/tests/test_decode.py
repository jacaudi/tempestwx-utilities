"""Tests for radar.decode: NEXRAD Level 3 NIDS bytes -> contoured GeoJSON (Contract A)."""

from pathlib import Path

import pytest

import decode

FIXTURE = Path(__file__).parent / "fixtures" / "TLX_N0B.nids"


def test_decode_bad_bytes_raises():
    with pytest.raises(decode.DecodeError):
        decode.to_geojson(b"not-nids", "N0B")


def test_decode_nids_to_isobands():
    result = decode.to_geojson(FIXTURE.read_bytes(), "N0B")

    assert result["type"] == "FeatureCollection"
    features = result["features"]
    assert len(features) > 0

    for feature in features:
        props = feature["properties"]
        assert props["dbz_min"] % 5 == 0
        assert props["dbz_max"] % 5 == 0
        assert props["dbz_max"] > props["dbz_min"]

    metadata = result["metadata"]
    assert metadata["site"] == "TLX"
    assert metadata["product"] == "N0B"
    assert metadata["scan_time"].endswith("Z")
    assert len(metadata["bbox"]) == 4
