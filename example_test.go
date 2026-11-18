package rls_test

import (
	"fmt"

	"github.com/autobrr/rls"
)

func ExampleParse() {
	const title = "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR"
	r := rls.ParseString(title)
	fmt.Printf("%q:\n", r)
	fmt.Printf("  type: %s\n", r.Type)
	fmt.Printf("  artist: %s\n", r.Artist)
	fmt.Printf("  title: %s\n", r.Title)
	fmt.Printf("  year: %d\n", r.Year)
	fmt.Printf("  disc: %s\n", r.Disc)
	fmt.Printf("  source: %s\n", r.Source)
	fmt.Printf("  edition: %q\n", r.Edition)
	fmt.Printf("  other: %q\n", r.Other)
	fmt.Printf("  group: %s\n", r.Group)
	// Output:
	// "The_Velvet_Underground-The_Complete_Matrix_Tapes-Reissue_Limited_Edition_Boxset-8LP-2019-NOiR":
	//   type: music
	//   artist: The Velvet Underground
	//   title: The Complete Matrix Tapes
	//   year: 2019
	//   disc: 8x
	//   source: LP
	//   edition: ["Limited.Edition"]
	//   other: ["REISSUE" "BOXSET"]
	//   group: NOiR
}

func ExampleDynamicRange() {
	r := rls.ParseString("Movie.2020.2160p.WEB-DL.HDR10.DV.HEVC-GRP")
	fmt.Println(r.DynamicRange)
	fmt.Printf("%q\n", r.DynamicRange.Strings())
	fmt.Println(r.DynamicRange.Has(rls.DV))
	fmt.Println(r.DynamicRange.Has(rls.DV | rls.HDR10Plus))
	// Output:
	// DV,HDR10
	// ["DV" "HDR10"]
	// true
	// false
}

func ExampleFlags() {
	r := rls.ParseString("Movie.2020.REAL.PROPER.REPACK2.1080p.BluRay.x264-GRP")
	fmt.Printf("%q\n", r.Other)
	fmt.Println(r.Flags)
	fmt.Println(r.Flags.Has(rls.FlagProper))
	fmt.Println(r.Repack)
	// Output:
	// ["REAL.PROPER" "REREPACK"]
	// PROPER,REAL
	// true
	// 2
}

func ExampleSource_Kind() {
	r := rls.ParseString("Movie.2020.2160p.UHD.WEB-DL.DV.HDR10.H265-GRP")
	fmt.Println(r.Source, r.Source.Kind() == rls.SourceWebDL, r.Source.UHD())
	fmt.Println(r.Codec[0], r.Codec[0].Kind() == rls.CodecHEVC)
	fmt.Println(r.Resolution, r.Resolution.Height(), r.Resolution.Interlaced())
	// Output:
	// UHD.WEB-DL true true
	// H.265 true
	// 2160p 2160 false
}
