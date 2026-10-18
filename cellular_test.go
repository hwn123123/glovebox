package main

import "testing"

func TestParseCellHelper(t *testing.T) {
	raw := "MODE=automatic\nCOUNT=1\nCELL\ttype=NR\tregistered=true\tconnection=0\tmcc=460\tmnc=00\toperator=China Mobile\tcell_id=43031085057\ttac=1048880\tpci=81\tarfcn=504990\tbands=41\tdbm=-94\trsrp=-94\trsrq=-11\tsinr=13\n"
	got, err := parseCellHelper(raw)
	if err != nil || len(got.Cells) != 1 {
		t.Fatalf("parseCellHelper = %#v, %v", got, err)
	}
	c := got.Cells[0]
	if got.SelectionMode != "automatic" || !c.Registered || c.Type != "NR" || c.PCI != "81" || c.ARFCN != "504990" || c.Bands != "41" || c.RSRP != -94 {
		t.Fatalf("unexpected cell: %#v / mode %q", c, got.SelectionMode)
	}
}

func TestParseCarrierAggregationRejectsHyperOSPlaceholders(t *testing.T) {
	raw := `mPhysicalChannelConfigs=[{mConnectionStatus=PrimaryServing,mCellBandwidthDownlinkKhz=0,mCellBandwidthUplinkKhz=0,mNetworkType=UNKNOWN,mFrequencyRange=UNKNOWN,mDownlinkChannelNumber=0,mUplinkChannelNumber=0,mContextIds=[0, 1],mPhysicalCellId=0,mBand=0,mDownlinkFrequency=0,mUplinkFrequency=0}, {mConnectionStatus=SecondaryServing,mCellBandwidthDownlinkKhz=0,mCellBandwidthUplinkKhz=0,mNetworkType=UNKNOWN,mFrequencyRange=UNKNOWN,mDownlinkChannelNumber=0,mUplinkChannelNumber=0,mContextIds=[0, 1],mPhysicalCellId=0,mBand=0,mDownlinkFrequency=0,mUplinkFrequency=0}]`
	cells := []cellularCell{{Type: "NR", Registered: true, PCI: "81", ARFCN: "504990", Bands: "41"}}
	got := parseCarrierAggregation(raw, cells)
	if len(got) != 1 || got[0].Source != "registered_cell" || got[0].PCI != "81" {
		t.Fatalf("aggregation = %#v", got)
	}
}

func TestParseCarrierAggregationUsesRealComponents(t *testing.T) {
	raw := `mPhysicalChannelConfigs=[{mConnectionStatus=PrimaryServing,mCellBandwidthDownlinkKhz=100000,mCellBandwidthUplinkKhz=100000,mNetworkType=NR,mDownlinkChannelNumber=504990,mUplinkChannelNumber=504990,mPhysicalCellId=81,mBand=41}]`
	got := parseCarrierAggregation(raw, nil)
	if len(got) != 1 || got[0].Band != "41" || got[0].DownlinkBandwidth != 100000 {
		t.Fatalf("aggregation = %#v", got)
	}
}
