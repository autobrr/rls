package rls

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/autobrr/rls/taginfo"
)

// TestZoneTable pins what the zone stage decides, cell by cell: for a word of
// a name, whether it is a word of a title or the tag a lexer claimed. A row
// gives the name, the word as it is written, and the type its tag has in the
// release. A zone that starts with 'cost' holds the names a rule reads wrong
// by what they mean, so that a change to the rule shows what it buys back.
func TestZoneTable(t *testing.T) {
	// a language directly in front of a year, which langBeforeYear decides
	cell := TagTypeLanguage
	if langBeforeYear {
		cell = TagTypeText
	}
	for _, test := range []struct {
		zone, s, word string
		typ           TagType
	}{
		// the first word of a name is a word, whatever it reads as
		{"first", "Web.Therapy.S01E01.720p.HDTV.x264-GRP", "Web", TagTypeText},
		{"first", "DOC.Martin.S01E01.720p.HDTV.x264-GRP", "DOC", TagTypeText},
		{"first", "French.Kiss.1995.1080p.BluRay.x264-GRP", "French", TagTypeText},
		{"first", "RED.2010.1080p.BluRay.x264-GRP", "RED", TagTypeText},
		{"first", "[SubsPlease] Complete Manual - 04 (1080p)", "Complete", TagTypeText},
		{"first", "(Comedy) Dave Chappelle - The Age of Spin (2017) 1080p WEBRip", "Comedy", TagTypeGenre},
		{"first", "[BD 1080p] Show Title - 01", "BD", TagTypeSource},
		// inside the title a weak claim is a word
		{"title", "The.French.Dispatch.2021.1080p.BluRay.x264-GRP", "French", TagTypeText},
		{"title", "The.Dutch.Master.S01E01.720p.HDTV.x264-GRP", "Dutch", TagTypeText},
		{"title", "A.Complete.Unknown.2024.1080p.WEB.h264-GRP", "Complete", TagTypeText},
		// in front of a year
		{"edge, year", "5.Headed.Shark.Attack.German.2017.1080p.BluRay.x264-GRP", "German", cell},
		{"edge, year", "Amelie French (2001) [1080p]", "French", cell},
		{"edge, year", "The.Good.German.2006.GERMAN.DL.1080p.BluRay.x264-GRP", "German", cell},
		{"edge, year", "Dracula.Tot.aber.gluecklich.German.1995.German.DL.1080p.BluRay.x265-PaTrol", "German", cell},
		{"edge, year", "Movie.Name.French.2019.1080p.BluRay.x264-GRP", "French", cell},
		// a language in front of a year is in that cell whatever follows the
		// year
		{"edge, year", "Stargate.SG1.German.2003.S7.D1.COMPLETE.PAL.DVDR-DAW", "German", cell},
		{"edge, year", "Sex.And.The.City.German.1998.Season.1.Disc1.Complete.DVDR-BTR", "German", cell},
		{"edge, year", "BBC.Verborgene.Welten.Das.geheime.Leben.der.Insekten.German.2006.E05.Der.Insektenstaat.DOKU.DVDRiP.XViD-DARM", "German", cell},
		{"edge, year", "Made.in.USA.1966.1080p.BluRay.x264-USURY", "USA", TagTypeText},
		{"edge, year", "Almost.Complete.2018.1080p.WEB.h264-GRP", "Complete", TagTypeText},
		{"edge, year", "Magnum.Opus.2017.1080p.WEB-DL.DD5.1.H264-GRP", "Opus", TagTypeText},
		{"edge, year", "The.Darjeeling.Limited.2007.1080p.BluRay.x264-CiNEFiLE", "Limited", TagTypeText},
		{"edge, year", "Night.1.REPACK.2023.1080p.WEB.h264-GRP", "REPACK", TagTypeOther},
		{"edge, year", "Some.Movie.EXTENDED.2010.1080p.BluRay.x264-GRP", "EXTENDED", TagTypeCut},
		{"edge, year", "Some.Movie.UNCUT.German.2016.1080p.BluRay.x264-GRP", "German", TagTypeLanguage},
		{"edge, year", "House MD (2004) Season 1 [1080p]", "MD", TagTypeText},
		{"edge, year", "El.Mago.Pop.Lands.in.USA.2024.SPANiSH.DOC.1080p.WEB.h264-SOMBRA", "USA", TagTypeText},
		// the spelling says nothing in a title of capitals, of small letters
		// or of digits, and the claim is a tag
		{"edge, capitals", "THE.MOVIE.PROPER.2017.1080p.BluRay.x264-GRP", "PROPER", TagTypeOther},
		{"edge, capitals", "THE.OFFICE.GERMAN.S01E01.720p.HDTV.x264-GRP", "GERMAN", TagTypeLanguage},
		{"edge, capitals", "2.22.3D.2008.German.DL.1080p.BluRay.x264-MAJESTiC", "3D", TagTypeOther},
		{"edge, capitals", "the.movie.proper.2017.1080p.bluray.x264-grp", "proper", TagTypeOther},
		{"edge, capitals", "going.dutch.s01e01.720p.hdtv.x264-grp", "dutch", TagTypeLanguage},
		{"edge, capitals", "MADE.IN.USA.1966.1080p.BluRay.x264-GRP", "USA", TagTypeRegion},
		{"edge, capitals", "Angora.By.Night.DANiSH.Season2.Disc1.PAL.DVDR-DROiDS", "DANiSH", TagTypeLanguage},
		// what the table costs: a tag written as a word in front of the year
		// or the numbering
		{"cost, edge", "The.Handmaiden.Extended.2016.Korean.Edition.COMPLETE.BLURAY-JRP", "Extended", TagTypeText},
		{"cost, edge", "Blood.Vengeance.For.Blood.Live.Limited.2006.JAPANESE.DVDRip.XviD-ASSASS1NS", "Limited", TagTypeText},
		{"cost, edge", "History.Of.Hardcore.Presented.By.DJ.Gizmo.Complete.2004.DUTCH.PAL.DVDr-HBDVD", "Complete", TagTypeText},
		{"cost, edge", "Show.Dutch.Staffel.1.1080p.WEB.h264-GRP", "Dutch", TagTypeText},
		// in front of the numbering, also behind its year
		{"edge, numbering", "Going.Dutch.S01E01.1080p.WEB.h264-ETHEL", "Dutch", TagTypeText},
		{"edge, numbering", "The.English.S01E01.1080p.BluRay.x264-POPPiNS", "English", TagTypeText},
		{"edge, numbering", "Murder.Most.English.S01E01.DVDRip.XviD-OSiTV", "English", TagTypeText},
		{"edge, numbering", "The.English.S01.NORDiC.SUBPACK.BluRay-REQ", "English", TagTypeText},
		// German in front of a numbering written in German is the language
		{"edge, numbering", "Robocop.German.Staffel1.FS.PAL.DVDR-HaNc0cK", "German", TagTypeLanguage},
		{"edge, numbering", "Show.German.Staffel.1.1080p.WEB.h264-GRP", "German", TagTypeLanguage},
		{"edge, numbering", "The.Office.US.S01E01.720p.HDTV.x264-GRP", "US", TagTypeRegion},
		{"edge, numbering", "Ghosts.US.2021.S01E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-FLUX", "US", TagTypeRegion},
		{"edge, numbering", "Hotellet.DANiSH.S01-S05.720p.WEB-DL.H.264", "DANiSH", TagTypeLanguage},
		{"edge, numbering", "Naruto Shippuden Dubbed S01E01 720p WEB x264-GRP", "Dubbed", TagTypeLanguage},
		{"edge, numbering", "Prim.and.Proper.S01E01.720p.WEB.h264-GRP", "Proper", TagTypeText},
		{"edge, numbering", "Some.Show.COMPLETE.S01.1080p.WEB.h264-GRP", "COMPLETE", TagTypeOther},
		// with a word behind it in front of the anchor a claim is inside the
		// title, whatever brackets are between. What stands directly in front
		// of a bracket and names no language is a tag all the same
		{"title, bracket", "[Group] Title in Russian (Other Title) - 04 [1080p]", "Russian", TagTypeText},
		{"title, bracket", "Buzzcocks-French_(US_Version)-1995-NuHS", "French", TagTypeText},
		{"title, bracket", "Lebedev_(RU)-English_Accent-RFCT025-WEB-2017-PITY", "English", TagTypeText},
		{"title, bracket", "Slavyanka_(Paul_Andrews)--Russian_Church_Music-1992-i8", "Russian", TagTypeText},
		{"title, bracket", "[Group] Title Complete (01-12) [1080p]", "Complete", TagTypeOther},
		{"title, bracket", "[drgn] Cowboy Bebop Complete (Series + Specials + Movies) [Dual Audio] [x265 HEVC 10 bit] [BD] [1080p]", "Complete", TagTypeOther},
		{"title, bracket", "Mastermind-Song_for_the_New_World-Remastered-(Japanese_Release)-2004-NTH", "Remastered", TagTypeOther},
		{"title, bracket", "Back Arrow + Extras [npz][US BD REMUX, 1080p] (Uncensored)", "Extras", TagTypeOther},
		// a claim that is all the title behind a hyphen is the title, and so
		// is one that shares a field of a record with the bracket
		{"title, bracket", "Artist-Remastered_(Live)-2003-GRP", "Remastered", TagTypeText},
		{"title, bracket", "Artist - Remastered (Live) (2011) [FLAC]", "Remastered", TagTypeText},
		{"title, bracket", "Artist-OST_(Original_Score)-WEB-2019-GRP", "OST", TagTypeText},
		{"title, bracket", "Artist-Dubbed_(Live)-2003-GRP", "Dubbed", TagTypeText},
		{"title, bracket", "Artist-The_Complete_(Studio_Recordings)-WEB-2019-GRP", "Complete", TagTypeText},
		{"title, bracket", "ananda_project--re-release_(japanese_edition)-cd2001-kw", "re-release", TagTypeText},
		{"title, bracket", "VA_-_Lilo_and_Stitch_OST_(Spanish_Ed.)-2002-EdeNMp3", "OST", TagTypeText},
		// a language in front of a bracket with no word behind it is a tag,
		// but for the year in parentheses
		{"edge, bracket", "Movie Title German (DL) 2019 1080p BluRay x264-GRP", "German", TagTypeLanguage},
		{"edge, bracket", "Show.Name.German.(S01).COMPLETE", "German", TagTypeLanguage},
		// a language that shares a field of a scene record with a word
		{"field", "Fabri_Fibra-Speak_English-x264-IT-2009-MVHiTS", "English", TagTypeText},
		{"field", "Missy_Elliot-Pass_That_Dutch-ivtc-Mvrs-Svcd-2003-mV4U-int", "Dutch", TagTypeText},
		{"field", "A2C-White_Russian-(PBR002)-WEB-2010-MPX", "Russian", TagTypeText},
		{"field", "Autoflo-Old_English-2004-FTD", "English", TagTypeText},
		{"field", "Hell_Driver-Excuse_My_English_EP-WEB-2016-XTC", "English", TagTypeText},
		{"field", "Losoul-Broken_English_Dubs-(LOG89)-WEB-2026-TR", "English", TagTypeText},
		{"field", "Polizei-Dutch_EP_(Rotterdam_Remix)_(TECHNO_12166)-Vinyl-1997-DGN", "Dutch", TagTypeText},
		{"field", "CCO-Italian_EP_(Part_2)-(HOSHI009)-WEB-2008-iTALiVE", "Italian", TagTypeText},
		{"field", "VA_-_Inselparty_Feten_-_Hits_GERMAN-2CD-2000-TLT", "GERMAN", TagTypeLanguage},
		{"field", "Acda_en_de_Munnik-Deel_2_Life_is_what_happens_to_you_DUTCH-2CD-2002-3G", "DUTCH", TagTypeLanguage},
		{"field", "EA_Sports_FC_24_In-Game_Commentary_Russian_DLC_NSW-SUXXORS", "Russian", TagTypeLanguage},
		{"field", "Wer_wird_Millionaer_PAL_GERMAN_PLUS4_PAL_NTSC_Selector-PARADOX", "GERMAN", TagTypeLanguage},
		{"field", "Der.Titel.German.DVDRip.XviD-GRP", "German", TagTypeLanguage},
		{"field", "Articolo_31_-_Italiano_Medio-Retail_CDA-IT-2003-WSC", "Retail", TagTypeOther},
		// with an edition claim behind it in the field the language says
		// where the edition is from, and it is the language when the tags
		// name it again
		{"field", "Nine_Inch_Nails-The_Downward_Spiral_Japanese_Import-CD-FLAC-1994-FLACME", "Japanese", TagTypeLanguage},
		{"field", "Azymuth-Brazilian_Soul_Japanese_Retail-2004-MVP", "Japanese", TagTypeLanguage},
		{"field", "Christina_Aguilera-Just_Be_Free_German_Import_CDM-2000-AUD", "German", TagTypeLanguage},
		{"field", "Zebrahead-MFZB_Japanese_Bonus_Tracks-2003-iTS", "Japanese", TagTypeLanguage},
		{"field", "Artist-Title_Russian_Reissue-WEB-2019-GRP", "Russian", TagTypeLanguage},
		{"field", "Artist-Title_French_CDM-2019-GRP", "French", TagTypeLanguage},
		{"field", "Artist-Some_Album_German-DE-WEB-2019-GRP", "German", TagTypeLanguage},
		{"field", "Henrik_Davidsson_-_The_Fenix_Year_Love_Loss_And_Finding_Home_Swedish-AUDiOBOOK-WEB-SE-2025-CRAViNGS_iNT", "Swedish", TagTypeLanguage},
		{"field", "VA-Hamilton_The_German_EP-DE-EP-16BIT-WEB-FLAC-2023-FLACED", "German", TagTypeText},
		// right after the year or the numbering, with two words behind
		{"episode title", "Show.S01E02.NO.WAY.OUT.1080p.WEB.h264-GRP", "NO", TagTypeText},
		{"episode title", "Formula1.2026.Hungarian.Grand.Prix.1080p.HDTV.x264-GRP", "Hungarian", TagTypeText},
		{"episode title", "Show.S01E02.GERMAN.1080p.WEB.h264-GRP", "GERMAN", TagTypeLanguage},
		{"episode title", "Austin.City.Limits.S48E02.Japanese.Breakfast-Arlo.Parks.1080p.WEB.h264-BAE", "Japanese", TagTypeText},
		{"episode title", "[A&C] One Piece Season 12 - Amazon Lily [408-421] (WEB) [Multi-Audio-Subs]", "Amazon", TagTypeText},
		// a bracket is a wall: what stands in it is no neighbour of a word.
		// A claim with the word that belongs to it is a tag in any bracket
		{"bracket, pair", "[Yameii] Clevatess - S02E02 [English Dub] [CR WEB-DL 720p H264 AAC]", "English", TagTypeLanguage},
		{"bracket, pair", "[ShinDora Subs] Doraemon 699 [English Sub]", "English", TagTypeLanguage},
		{"bracket, pair", "[Ohys-Raws] Strike the Blood [Unofficial English Subtitle Scripts][KOTEX]", "English", TagTypeLanguage},
		{"bracket, pair", "[CBM] Persona 3 the Movie 1-4 Complete (English Subtitled) [BDRip 1080p H.265 10bit]", "English", TagTypeLanguage},
		{"bracket, pair", "[BDMV] 07-Ghost {English Subbed, Missing Last Ep & No Eng Dub} (US, Discotek) [1080i]", "English", TagTypeLanguage},
		// a bracket of tags holds tags, and so does one from its first
		// source, audio or technical tag on
		{"bracket, tags", "[KIRI] Azure Compass - 1157 [Web][MKV][h264][1080p]", "Web", TagTypeSource},
		{"bracket, tags", "{QuQu's} [R] No Game No Life (English-DUB) + Movie [1080p]", "English", TagTypeLanguage},
		// a language and the claim behind it are read as one where the
		// bracket stands: words inside the title, tags behind a tag or with
		// nothing but tags behind them
		{"bracket, tags", "LTJ_Bukem-Horizons_(Japanese_Import)-SRCS300-1997-sour", "Japanese", TagTypeText},
		{"bracket, tags", "LTJ_Bukem-Horizons_(Japanese_Import)-SRCS300-1997-sour", "Import", TagTypeText},
		{"bracket, tags", "VA-The_Cheetah_Girls_2-(Italian_Bonus_Track)-OST-IT-2006-SAW", "Bonus_Track", TagTypeText},
		{"bracket, tags", "LTJ_Bukem-Mystical_Realms_EP_(Japanese_Import)-SRCS324-1998-sour", "Japanese", TagTypeLanguage},
		{"bracket, tags", "LTJ_Bukem-Mystical_Realms_EP_(Japanese_Import)-SRCS324-1998-sour", "Import", TagTypeOther},
		{"bracket, tags", "X_TV-OST_(Limited_Edition_Bonus_Japanese_Import)-JP-2002-LSi", "Japanese", TagTypeLanguage},
		{"bracket, tags", "Annihilator-Schizo_Deluxe-(Ltd_Russian_Reissue)-2006-VERiTAS_INT", "Russian", TagTypeLanguage},
		{"bracket, tags", "[Group] Show - 05 (BD 1080p HEVC English Judas)", "English", TagTypeLanguage},
		{"bracket, tags", "[ProRemux666] Cleopatra D.C. (1989) - OVA (JPN Blu-ray Remux | 1080p x264 | FLAC 2.0 | English / .ASS)", "English", TagTypeLanguage},
		// a claim alone in its bracket is a tag. A language alone is read
		// where the bracket stands, but not next to a bracket of tags
		{"bracket, lone", "[Trix] 86 (Eighty-Six) Season 1 (Part 1+2) (COMPLETE) [Optional Dual Audio][Multi Subs] (BD 1080p AV1) - VOSTFR", "COMPLETE", TagTypeOther},
		{"bracket, lone", "Glitter Force Doki Doki [Netflix] 1- 30 [720p]", "Netflix", TagTypeCollection},
		{"bracket, lone", "Toto_Cutugno-Un_Italiano_Romantico-(Retail)-IT-1998-R3D", "Retail", TagTypeOther},
		{"bracket, lone", "Uma Musume Cinderella Gray S01 1080p BDRip DDP5.1 x264 (Vol 5) (Multi-Subs) (Umamusume: Cinderella Gray)", "Multi-Subs", TagTypeLanguage},
		{"bracket, lone", "Goldorak.S01Part01.MULTI.Audio.Sub[FRE][ENG][ITA][ARA].1080p.BDRip.x264-KuroNeko", "ENG", TagTypeLanguage},
		{"bracket, lone", "German_Martinez_(DE)-Surgery_In_Los_Rios-(XR283)-WEB-2024-PTC", "DE", TagTypeText},
		{"bracket, lone", "Kraftwerk_-_The_Man_Machine_(English)_Track_1_Fixed-1978-CMG", "English", TagTypeText},
		// a claim with a word behind it in a bracket of words is a word of a
		// phrase. So is a language with a word in front of it, in front of
		// the anchor
		{"bracket, phrase", "After_School-Bang_(Japanese_Version)-JA-DVDRIP-RETAIL-x264-2011-FRAY", "Japanese", TagTypeText},
		{"bracket, phrase", "2_Pac-Me_Against_the_World_(German_Edition)-Interscope-CDM-1995-BFHMP3", "German", TagTypeText},
		{"bracket, phrase", "Agnes_Obel-Riverside_(Danish_Music_Awards_2011)-x264-2011-GRMV", "Danish", TagTypeText},
		{"bracket, phrase", "Sonata_Arctica-Silence-(Japanese_Release)-2001-SF", "Japanese", TagTypeText},
		{"bracket, phrase", "Copywrite-God_Save_The_King_(Proper_English_Version)-WEB-2012-UVU", "Proper", TagTypeText},
		{"bracket, phrase", "Artist - Album (Remastered Version) (2019) [FLAC]", "Remastered", TagTypeText},
		{"bracket, phrase", "VA-Progressions_01_London_(Mixed_by_Max_English)-CD-2001-tronik", "English", TagTypeText},
		{"bracket, phrase", "[Judas] Tonari no Alya-san (Alya Sometimes Hides Her Feelings in Russian) (Season 01) [BD 1080p]", "Russian", TagTypeText},
		{"bracket, phrase", "[krishna009] Naruto & Naruto Shippuuden Complete Cut To Manga (AKA Anime with NO FILLER)", "NO", TagTypeText},
		{"bracket, phrase", "[RH] Kaiba - NCOP (English Title Ver) [152C994A].mkv (BDRip)", "English", TagTypeText},
		{"bracket, phrase", "All_Shall_Perish-The_Price_Of_Existence-(Korean_Bonus_Tracks)-2006-RAN", "Bonus_Tracks", TagTypeOther},
		{"bracket, phrase", "Nirvana-Box_(Chinese_Retail)-3CD-2003-x3m", "Retail", TagTypeOther},
		{"bracket, phrase", "VA_-_Thunderdome_97-2CD-(Dutch_Edition)-1997-KLF", "Dutch", TagTypeText},
		{"bracket, phrase", "Pleymo-Episode_2_Medecine_Cake_(Japanese_Release)-FR-2002-FSP", "Japanese", TagTypeText},
		{"bracket, phrase", "[Hatsuyuki]_07-Ghost_-_01[1280x720][Vietnamese_English]", "English", TagTypeLanguage},
		// in front of the tags a bracket lists, a word makes a phrase only in
		// front of the anchor. Behind it a claim is a tag unless it stands
		// between two words
		{"bracket, head", "Ten-The_Robe_(Japanese_Release_CDS)-1997-AMRC", "Japanese", TagTypeText},
		{"bracket, head", "[NC-Raws] The Soul of Soldier Master - 14 (B-Global Donghua 1920x1080 HEVC AAC MKV)", "B-Global", TagTypeCollection},
		{"bracket, head", "BEASTARS S03 1080p NF WEB-DL DUAL DDP5.1 H 264-VARYG (BEASTARS FINAL SEASON Part 2, Dual-Audio, Multi-Subs)", "FINAL", TagTypeText},
		// what the bracket rules cost: a phrase that says the release is
		// complete, and a language in a bracket of tags that says where a
		// reissue is from, with nothing but tags behind the bracket
		{"cost, bracket", "Death Note (Complete Series) [1080p] (Dual Audio)", "Complete", TagTypeText},
		{"cost, bracket", "[RigAV1] Saenai Heroine no Sodatekata S01+S02+OVAs+Movie (COMPLETE Series) (BD 1080p AV1 Opus)", "COMPLETE", TagTypeText},
		{"cost, bracket", "Adolf_Castle-Really_Crazy_Germans-(Russian_Reissue)-1999-VERiTAS_INT", "Russian", TagTypeLanguage},
		// with no word in front of the anchor, the collection that opens the
		// name is its title
		{"no title", "Hulu.2019.1080p.WEB.h264-GRP", "Hulu", TagTypeText},
		{"no title", "XXX.2002.NORDIC.PAL.DVDR.SUB.FIX-EFTERLYST", "XXX", TagTypeText},
		{"no title", "DSNP.S01E01.1080p.WEB.h264-GRP", "DSNP", TagTypeText},
		{"no title", "The.Criterion.2019.1080p.BluRay.x264-GRP", "The.Criterion", TagTypeText},
		{"no title", "(Uncut) 2019", "Uncut", TagTypeText},
		{"no title", "[CC] 2019 1080p", "CC", TagTypeText},
		{"no title", "[KIRI] Azure Compass - 1157 [Web][MKV][h264][1080p]", "Web", TagTypeSource},
		// between two words
		{"between", "Tennis.US.Open.2025.1080p.HDTV.x264-GRP", "US", TagTypeText},
		{"between", "Canada.Vs.USA.1080p.HDTV.x264-GRP", "USA", TagTypeText},
		{"between", "Ajax.vs.PSV.720p.HDTV.x264-GRP", "PSV", TagTypeText},
		{"between", "Arsenal.vs.Liverpool.Extended.Highlights.1080p.HDTV.x264-GRP", "Extended", TagTypeText},
		{"between", "Show.S03E03.A.Dark.Web.1080p.WEBRip.x264-GRP", "Web", TagTypeText},
		// rows that are tags wherever they stand
		{"keeps", "Fumoon.1980.Special.MULTI.audio.sub.1080p.BDRip.CUSTOM.x264.PCM-KuroNeko", "MULTI", TagTypeLanguage},
		{"keeps", "Formula1.2024.Japan.Qualifying.F1TV.Live.1080p.WEB.h264-GRP", "F1TV", TagTypeCollection},
		// what the name as a whole decides before the zones are read
		{"windows", "Broken.Windows.2020.1080p.WEB.h264-GRP", "Windows", TagTypeText},
		{"windows", "Some.Title-GROUP (2019) [Windows / Multi-Language / Full ISO]", "Windows", TagTypePlatform},
		{"dates", "2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GRP", "2001", TagTypeText},
		{"dates", "2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GRP", "1968", TagTypeDate},
		{"dates", "2012.1080p.BluRay.x264-GRP", "2012", TagTypeText},
		{"range", "[Erai-raws] Tatsuki Fujimoto 17-26 - 01 ~ 08 [1080p]", "17-26", TagTypeText},
		{"range", "[Erai-raws] Show 01-12 - 13 [1080p]", "01-12", TagTypeSeries},
		{"range", "[Erai-raws] Show 17-26 - 01 ~ 08 | Other 27-36 - 09 ~ 16 [1080p]", "27-36", TagTypeText},
		// what the type decides
		{"video", "The.CD.2020.1080p.WEB.H264-GRP", "CD", TagTypeText},
		{"video", "Show.S01E02.DVB.x264-GRP", "DVB", TagTypeSource},
		{"video", "CNBC.Titans.S01E01.1080p.AMZN.WEB-DL.DDP2.0.H.264-GRP", "CNBC", TagTypeText},
		{"video", "F1TV.Formula1.2024.Round19.USA.Race.1080p.WEB.h264-GRP", "F1TV", TagTypeCollection},
	} {
		tags := ParseString(test.s).Tags()
		i := slices.IndexFunc(tags, func(tag Tag) bool { return fmt.Sprintf("%o", tag) == test.word })
		switch {
		case i == -1:
			t.Errorf("%s: %q has no tag %q: %q", test.zone, test.s, test.word, tags)
		case tags[i].TagType() != test.typ:
			t.Errorf("%s: %q expected %q to be %s, got: %s", test.zone, test.s, test.word, test.typ, tags[i].TagType())
		}
	}
}

// TestComputeZones checks the zones of a name by the words they fall on.
func TestComputeZones(t *testing.T) {
	for _, test := range []struct {
		s                                 string
		start, last, close, anchor, dated string
	}{
		{"The.French.Dispatch.2021.1080p.BluRay.x264-GRP", "The", "Dispatch", "2021", "2021", "2021"},
		{"[SubsPlease] Hell Mode S2 - 04 (480p) [20BE14E8].mkv", "Hell", "Mode", "S2", "S2", "04"},
		{"[BD 1080p] Show Title - 01 [Group]", "Show", "Title", "01", "01", "01"},
		{"(Comedy) Dave Chappelle - The Age of Spin (2017) 1080p WEBRip", "Dave", "Spin", " (", "2017", "2017"},
		{"[Erai-raws] Tatsuki Fujimoto 17-26 - 01 ~ 08 [1080p]", "Tatsuki", "17-26", "01 ~ 08", "01 ~ 08", "01 ~ 08"},
		{"Artist-Album-CD-FLAC-2019-GRP", "Artist", "Album", "CD", "CD", "2019"},
		{"Some.Tool.MacOS-GRP", "Some", "Tool", "", "", ""},
	} {
		tags, end := ParseTagsString(test.s)
		z := computeZones(tags, end)
		at := func(i int) string {
			if i >= end {
				return ""
			}
			return tags[i].val(0)
		}
		if got := [...]string{at(z.start), at(z.last), at(z.close), at(z.anchor), at(z.dated)}; got != [...]string{test.start, test.last, test.close, test.anchor, test.dated} {
			t.Errorf("%q expected start, last, close, anchor and dated at %q, got: %q", test.s,
				[...]string{test.start, test.last, test.close, test.anchor, test.dated}, got)
		}
	}
}

// funcsWith returns, for each function of the package's source that is no
// test, how many nodes it holds that match reports. A function literal of a
// package variable counts under the name of the variable.
func funcsWith(t *testing.T, match func(ast.Node) bool) map[string]int {
	t.Helper()
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset, m := token.NewFileSet(), make(map[string]int)
	for _, entry := range files {
		if name := entry.Name(); !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		count := func(name string, body ast.Node) {
			ast.Inspect(body, func(n ast.Node) bool {
				if n != nil && match(n) {
					m[name]++
				}
				return true
			})
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Body != nil {
					count(d.Name.Name, d.Body)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if v, ok := spec.(*ast.ValueSpec); ok && len(v.Names) != 0 {
						for _, value := range v.Values {
							count(v.Names[0].Name, value)
						}
					}
				}
			}
		}
	}
	return m
}

// retypes reports whether n gives a tag the type typ, in one of the ways the
// package can write it: a call of As, as, NewTag or newTag with typ as its
// first argument ('tag.As(TagTypeText, nil)'), an assignment to the type of a
// tag ('tags[i].typ = TagTypeText'), or a tag written out with it, by name or
// by position ('Tag{typ: TagTypeText}', 'Tag{TagTypeText, nil, v}').
//
// It reads the source and not the program, so it cannot see a type that
// reaches one of these through a variable ('typ := TagTypeText; tag.As(typ,
// nil)'), a call through a function value, or a write through reflect or
// unsafe. The tests that use it know a function by its name alone: a method
// of another receiver with the name of an owner counts as that owner.
func retypes(n ast.Node, typ string) bool {
	is := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && id.Name == typ
	}
	switch n := n.(type) {
	case *ast.CallExpr:
		var name string
		switch f := n.Fun.(type) {
		case *ast.SelectorExpr:
			name = f.Sel.Name
		case *ast.Ident:
			name = f.Name
		}
		return slices.Contains([]string{"As", "as", "NewTag", "newTag"}, name) && len(n.Args) != 0 && is(n.Args[0])
	case *ast.AssignStmt:
		for i, lhs := range n.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "typ" && (len(n.Rhs) != len(n.Lhs) || is(n.Rhs[i])) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		key, ok := n.Key.(*ast.Ident)
		return ok && key.Name == "typ" && is(n.Value)
	case *ast.CompositeLit:
		// typ is the first field of Tag
		lit, ok := n.Type.(*ast.Ident)
		return ok && lit.Name == "Tag" && len(n.Elts) != 0 && is(n.Elts[0])
	}
	return false
}

// TestOneOwner checks that the zone stage is the one place that decides a
// word a lexer claimed is a word after all. A claim is given back as text
// only by the functions of the stage, and by those listed with the reason
// each is not the stage's question. See retypes for what it cannot see.
func TestOneOwner(t *testing.T) {
	stage := map[string]string{
		"settle":           "the stage",
		"settleDates":      "the stage",
		"settleWindows":    "the stage",
		"settleVideo":      "the stage",
		"musicSourceWords": "the stage",
		"unsettle":         "the stage, which makes words tags again",
		"fixAudioRange":    "a range that is the bit depth and sample rate of a record is no numbering, which is a question of what a number counts",
		"fixMusic":         "one episode in a name with a tag of a record is a track, which is a question of what a number counts",
		"unset":            "a row of another release type is no tag of this one, which the type decides for the whole name and no zone does",
		"fixTailGroup":     "a word of the group in the leading bracket, which goes with the group",
	}
	found := funcsWith(t, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if f, ok := call.Fun.(*ast.SelectorExpr); ok && f.Sel.Name == "Revert" {
				return true
			}
		}
		return retypes(n, "TagTypeText")
	})
	for name := range found {
		if _, ok := stage[name]; !ok {
			t.Errorf("%s gives a tag back as text, and is not the zone stage: decide it in zone.go, or list it here with the reason", name)
		}
	}
	for name := range stage {
		if found[name] == 0 {
			t.Errorf("%s is listed and gives no tag back as text: take it off the list", name)
		}
	}
}

// TestRetypes checks that retypes sees each way of writing a retype that
// TestOneOwner and TestGroupOwner rely on.
func TestRetypes(t *testing.T) {
	for src, exp := range map[string]bool{
		`tags[i] = tags[i].As(TagTypeText, nil)`:      true,
		`tags[i] = NewTag(TagTypeText, nil, a, b)`:    true,
		`r.tags[i].typ = TagTypeText`:                 true,
		`tag.typ, n = TagTypeText, 1`:                 true,
		`tag.typ, n = f()`:                            true,
		`tags[i] = Tag{typ: TagTypeText, v: v}`:       true,
		`tags[i] = Tag{TagTypeText, nil, v}`:          true,
		`tags[i] = Tag{TagTypeDelim, nil, v}`:         false,
		`tags[i] = tags[i].As(TagTypeDelim, nil)`:     false,
		`tags[i] = Tag{typ: TagTypeDelim, v: v}`:      false,
		`tags = append(tags, newTag(s, b, typ, nil))`: false,
		`n = tag.typ`: false,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "", "package p\nfunc f() {\n"+src+"\n}", 0)
		if err != nil {
			t.Fatal(err)
		}
		var got bool
		ast.Inspect(file, func(n ast.Node) bool {
			got = got || n != nil && retypes(n, "TagTypeText")
			return true
		})
		if got != exp {
			t.Errorf("%q expected retypes to report %t, got: %t", src, exp, got)
		}
	}
}

// TestTitleNotEmpty checks that a name whose only word in front of the year
// or the numbering is a claim keeps that word as its title: a collection that
// opens the name, and a spelling that is an ordinary word alone in a leading
// bracket. It holds with the default vocabulary, with a collection the caller
// adds, and with every type a caller can give, but for the word in a square
// bracket that an episode has for its group.
func TestTitleNotEmpty(t *testing.T) {
	const own = "Brave.2012.1080p.BluRay.x264-GRP"
	p := NewTagParser(taginfo.All(extraCollections(t, []string{"BRAVE", "Brave", "(?i:BRAVE)", "", "", ""})), DefaultLexers()...)
	if r := p.ParseRelease([]byte(own)); r.Title != "Brave" || r.Collection != "" {
		t.Errorf("%q with a collection BRAVE expected title %q and no collection, got: %q and %q", own, "Brave", r.Title, r.Collection)
	}
	for s, exp := range map[string]string{
		"XXX.2002.NORDIC.PAL.DVDR.SUB.FIX-EFTERLYST": "XXX",
		"Hulu.2019.1080p.WEB.h264-GRP":               "Hulu",
		"Starz.2019.1080p.WEB.h264-GRP":              "Starz",
		"NF.2019.1080p.WEB.h264-GRP":                 "NF",
		"DSNP.S01E01.1080p.WEB.h264-GRP":             "DSNP",
		"The.Criterion.2019.1080p.BluRay.x264-GRP":   "The Criterion",
		"VTC.S01E01.FRENCH.1080p.WEB.H264-PROPJOE":   "",
		// the same word alone in a bracket that opens the name
		"(Uncut) 2019":         "Uncut",
		"(Uncut) S01E01 1080p": "Uncut",
		"[CC] 2019 1080p":      "CC",
		"(MD) 2019":            "MD",
		"(Web) 2019":           "Web",
		"[RED] 2019":           "RED",
		"(Amazon) 2019 1080p":  "Amazon",
	} {
		if exp != "" {
			if r := ParseString(s); r.Title != exp {
				t.Errorf("%q expected title %q, got: %q", s, exp, r.Title)
			}
			if r := p.ParseRelease([]byte(s)); r.Title != exp {
				t.Errorf("%q with a collection BRAVE expected title %q, got: %q", s, exp, r.Title)
			}
		}
		for typ := Unknown + 1; typ <= Series; typ++ {
			r := ParseStringHint(s, Hint{Type: typ})
			// the word in the square bracket that opens an episode is its
			// group ('[SubsPlease] Show - 01')
			if s[0] == '[' && typ.Is(Episode, Series) {
				if r.Group != exp {
					t.Errorf("%q hinted %s expected group %q, got: %q", s, typ, exp, r.Group)
				}
				continue
			}
			if r.Title == "" && r.Artist == "" {
				t.Errorf("%q hinted %s expected a title, got none", s, typ)
			}
		}
	}
}

// TestSettleLinear checks that the time to parse a name grows with its length
// and not with the square of it, for names made of what the stage looks
// around for: claims, sources, brackets and numbering. A name is untrusted
// input. Each shape is parsed at two lengths, one eight times the other, and
// the longer may take 24 times as long: a pass over the name for each claim
// takes 64.
//
// The last shape, a name that is all numbering, is timed on the stage alone:
// the accessors of the numbering that collect calls walk the tags once for
// each numbering tag, which is not the stage's to answer for.
func TestSettleLinear(t *testing.T) {
	b := NewTagBuilder()
	clock := func(s string, stage bool) time.Duration {
		tags, end := ParseTagsString(s)
		best := time.Duration(1<<63 - 1)
		for range 5 {
			r := &Release{tags: slices.Clone(tags), end: end}
			start := time.Now()
			if stage {
				b.settle(r)
			} else {
				ParseString(s)
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	for _, shape := range []struct {
		head, unit, tail string
		stage            bool
	}{
		{"", "Web.", "S01E01.2017.1080p.x264-GRP", false},
		{"", "a.WEB.", "S01E01.2017.1080p.x264-GRP", false},
		{"Title.", "German.", "2017.1080p.x264-GRP", false},
		{"Title (", "Eng ", "x) S01E01 1080p x264-GRP", false},
		{"Title ", "[Eng]", " S01E01 1080p x264-GRP", false},
		{"Artist-a_", "Dub_Eng_", "x-WEB-2019-GRP", false},
		{"Artist-", "a_EP_", "x-WEB-2019-GRP", false},
		{"Artist-", "a_Complete_(x)_", "x-WEB-2019-GRP", false},
		{"", "(Uncut) ", "2019", false},
		{"", "Title 17-26 - 01 ~ 08 ", "", true},
	} {
		short := clock(shape.head+strings.Repeat(shape.unit, 1000)+shape.tail, shape.stage)
		long := clock(shape.head+strings.Repeat(shape.unit, 8000)+shape.tail, shape.stage)
		if long > 24*short {
			t.Errorf("%q: 8000 of %q took %s and 1000 took %s, more than 24 times as long", shape.head+shape.unit+shape.tail, shape.unit, long, short)
		}
	}
}
