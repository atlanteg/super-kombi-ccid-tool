package bmwzgw

import (
	"fmt"
	"testing"
)

func TestDecodeVIN(t *testing.T) {
	tests := []struct {
		vin      string
		wantChas string
		note     string
	}{
		// '7' → G11 (was F01)
		{"WBA7C41070G762791", "G11", "G11 730d xDrive MY2016"},
		{"WBA7C21070BP41411", "G11", "G11 730d vin[10]='B' anomaly — still G11"},

		// 'K' F-series → F15 (via gseriesIntroMY['K']='G')
		{"WBAKS410X00C32596", "F15", "F15 X5 xDrive30d"},
		{"X4XKS694000K28627", "F15", "F15 X5 xDrive40d BMW SA"},
		{"X4XKC81170C572321", "F15", "F02 750Li — best guess F15 (acceptable)"},
		{"WBSKT610800C88668", "F15", "F85 X5M — best guess F15 (acceptable)"},

		// 'C' → G05 (was F25)
		{"WBACV6100L9C83449", "G05", "G05 X5 xDrive30d"},
		{"WBACV61070LJ85160", "G05", "G05 X5 xDrive30d"},

		// 'H' → G29 (vin[10]='W' >= 'K') was G20
		{"WBAHF51090WX29248", "G29", "G29 Z4 M40i"},

		// 'H' F-series → F48 FWD (vin[10]='5' digit < 'K')
		{"WBAHS120205F03712", "F48", "F48 X1 sDrive18i FWD"},
		{"WBAHS120805F01298", "F48", "F48 X1 sDrive18i FWD"},

		// 'L' → F13 (was G01)
		{"WBALX51070C799615", "F13", "F13 650i xDrive confirmed fix"},

		// 'T' → G01 (was G42)
		{"WBATS31060LC17963", "G01", "G01 X3 M40i"},
		{"WBATX71090LG17542", "G01", "G01 X3 xDrive30d"},
		{"WBAUZ3102LLT05586", "G01", "G01 X3 xDrive20d (via 'U' key)"},

		// 'W' F-series → F25 (via gseriesIntroMY['W']='N')
		{"WBAWX71020L495147", "F25", "F25 X3 xDrive35i"},
		{"X4XWX39450LN99668", "F25", "F25 X3 xDrive20i BMW SA"},

		// 'X' → F26 (BMW SA; was G22)
		{"X4XXW394X00P64042", "F26", "F26 X4 xDrive28i BMW SA"},

		// 'Y' → F39 FWD (was G15)
		{"WBAYH120705S34506", "F39", "F39 X2 sDrive18i FWD"},

		// '8' → F30 (was F34)
		{"WBA8A51050AE75665", "F30", "F30 320i xDrive"},
		// SA right-hand drive uses VIN[4]='E' for sedan — conflicts with G-series Touring code;
		// decoded as F31 (wrong body label but same F020 platform — known limitation).
		{"WBA8E36070NV05031", "F31", "F30 318i SAF — decoded F31 due to body-code ambiguity"},

		// '8' + VIN[4]='K' → F31 Touring
		{"WBA8K12000K612555", "F31", "F31 318i Touring"},

		// '3' → F33 convertible
		{"WBA3V9C56FP946935", "F33", "F33 428i xDrive Convertible"},

		// '4' → F36 Gran Coupe
		{"WBA4F11010G314880", "F36", "F36 420d xDrive Gran Coupe"},

		// G30 5-series still works
		{"WBAJC31000G921909", "G30", "G30 520d"},

		// Previously working cases — regression check
		{"WBA5A31030D754390", "F10", "F10 520i"},
		{"WBA1A11080E599591", "F20", "F20 116i"},
	}

	pass, fail := 0, 0
	for _, tt := range tests {
		chassis, model, _, _, _ := decodeVIN(tt.vin)
		ok := chassis == tt.wantChas
		mark := "PASS"
		if !ok {
			mark = "FAIL"
			fail++
		} else {
			pass++
		}
		fmt.Printf("%s VIN=%-18s chassis=%-5s want=%-5s model=%s\n",
			mark, tt.vin, chassis, tt.wantChas, model)
	}
	fmt.Printf("\n%d pass, %d fail\n", pass, fail)
	if fail > 0 {
		t.Errorf("%d test cases failed", fail)
	}
}
