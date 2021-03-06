package rst

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindLocalRefs(t *testing.T) {
	cases := []struct {
		input    string
		expected []RefTarget
	}{{
		input:    "",
		expected: []RefTarget{},
	}, {
		input:    ".. _:",
		expected: []RefTarget{},
	}, {
		input:    ".. _foo:",
		expected: []RefTarget{{Name: "foo"}},
	}, {
		input:    ".. _foo:\n.. _bar:",
		expected: []RefTarget{{Name: "foo"}, {Name: "bar"}},
	}, {
		input:    ".. _foo:\n.. _bar:\n\n\n\n\n\n.. _baz:",
		expected: []RefTarget{{Name: "foo"}, {Name: "bar"}, {Name: "baz"}},
	}, {
		input:    ".. _version-4.1:",
		expected: []RefTarget{{Name: "version-4.1"}},
	}, {
		input:    ".. _mongodb-compatibility-table-about-{+driver+}:",
		expected: []RefTarget{{Name: "mongodb-compatibility-table-about-{+driver+}"}},
	}, {
		input:    "    - ..  _unionWith-coll:",
		expected: []RefTarget{{Name: "unionWith-coll"}},
	}, {
		input:    ".. _faq-storage limit:",
		expected: []RefTarget{{Name: "faq-storage limit"}},
	},
	}

	for _, c := range cases {
		actual := ParseForLocalRefs([]byte(c.input))
		assert.ElementsMatch(t, c.expected, actual, "ParseForLocalRefs(%q) should return %v, got %v", c.input, c.expected, actual)
	}

}

func TestConstantParser(t *testing.T) {

	cases := []struct {
		input    string
		expected []RstConstant
	}{{
		input:    "",
		expected: []RstConstant{},
	}, {
		input:    ".. _:",
		expected: []RstConstant{},
	}, {
		input:    ".. _: foo",
		expected: []RstConstant{},
	}, {
		input:    "This is a `constant link that should fail <{+api+}/flibbertypoo>`__",
		expected: []RstConstant{{Target: "/flibbertypoo", Name: "api"}},
	}, {
		input:    "This is a `constant link that should succeed <{+api+}/classes/AggregationCursor.html>`__",
		expected: []RstConstant{{Target: "/classes/AggregationCursor.html", Name: "api"}},
	}, {
		input:    "here is a :ref:`fantastic`",
		expected: []RstConstant{},
	}, {
		input:    "Here is one `constant link <{+api+}/One.html>`__ and a second `constant link <{+api+}/Two.html>`__",
		expected: []RstConstant{{Target: "/One.html", Name: "api"}, {Target: "/Two.html", Name: "api"}},
	},
	}
	for _, test := range cases {
		got := ParseForConstants([]byte(test.input))
		assert.ElementsMatch(t, test.expected, got, "ParseForConstants(%q) should return %v, got %v", test.input, test.expected, got)
	}
}

func TestFindLinkInConstant(t *testing.T) {
	cases := []struct {
		input    RstConstant
		expected bool
	}{{
		input:    RstConstant{Target: "https://www.google.com", Name: "api"},
		expected: true,
	}, {
		input:    RstConstant{Target: "v1.8.0", Name: "api"},
		expected: false,
	}}

	for _, c := range cases {
		actual := c.input.IsHTTPLink()
		assert.Equal(t, c.expected, actual, "IsLink(%q) should return %v, got %v", c.input, c.expected, actual)
	}
}

func TestLinkParser(t *testing.T) {
	cases := []struct {
		input    string
		expected []RstHTTPLink
	}{{
		input:    "",
		expected: []RstHTTPLink{},
	}, {
		input:    "\n\n\n",
		expected: []RstHTTPLink{},
	}, {
		input:    "// code comments \n /* and more comments */ \n // and yet more!",
		expected: []RstHTTPLink{},
	}, {
		input:    "we can say http and www without any links being found",
		expected: []RstHTTPLink{},
	}, {
		input:    "https://www.flibberptyquz.co",
		expected: []RstHTTPLink{"https://www.flibberptyquz.co"},
	}, {
		input:    "markdown links are found\n\t\t [some markdown link](https://www.google.com)\\n\" +\n\t\t\"   [some other link](https://a.bad.url)\\n\" +",
		expected: []RstHTTPLink{RstHTTPLink("https://www.google.com"), RstHTTPLink("https://a.bad.url")},
	}, {
		input:    "http links in rst are found\n\t\t\"   this is a bad `url <https://www.flibbertypip.com>`__\\n\" +\n\t\t\"   this is a good `url <https://www.github.com>`__",
		expected: []RstHTTPLink{RstHTTPLink("https://www.flibbertypip.com"), RstHTTPLink("https://www.github.com")},
	},
	}
	for _, test := range cases {
		got := ParseForHTTPLinks([]byte(test.input))
		assert.ElementsMatch(t, test.expected, got, "ParseForConstants(%q) should return %v, got %v", test.input, test.expected, got)
	}
}

//go:embed testdata/makesGoUnhappy.txt
var edge []byte

func TestRoleParser(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []RstRole
	}{{
		input:    []byte(""),
		expected: []RstRole{},
	}, {
		input:    []byte(".. _:"),
		expected: []RstRole{},
	}, {
		input:    []byte(".. _: foo"),
		expected: []RstRole{},
	}, {
		input:    []byte("This is a `constant link that should fail <{+api+}/flibbertypoo>`__"),
		expected: []RstRole{},
	}, {
		input:    []byte("This is a `constant link that should succeed <{+api+}/classes/AggregationCursor.html>`__"),
		expected: []RstRole{},
	}, {
		input:    []byte("here is a :ref:`fantastic`"),
		expected: []RstRole{{Target: "fantastic", RoleType: "ref", Name: "ref"}},
	}, {
		input:    []byte("here is a :ref:`fantastic` here is another :ref:`2 <mediocre-fantastic>` here is a :ref:`\n<not_great-fantastic>`"),
		expected: []RstRole{{Target: "fantastic", RoleType: "ref", Name: "ref"}, {Target: "mediocre-fantastic", RoleType: "ref", Name: "ref"}, {Target: "not_great-fantastic", RoleType: "ref", Name: "ref"}},
	}, {
		input:    []byte("Here is one `constant link <{+api+}/One.html>`__ and a second `constant link <{+api+}/Two.html>`__"),
		expected: []RstRole{},
	}, {
		input:    edge,
		expected: []RstRole{{Target: "/reference/operator/update/positional-filtered/", RoleType: "role", Name: "manual"}},
	}, {
		input:    []byte("here is a :ref:`fantastic`"),
		expected: []RstRole{{Target: "fantastic", RoleType: "ref", Name: "ref"}},
	}, {
		input:    []byte(":ref:`What information does the MongoDB Compatibility table show? <mongodb-compatibility-table-about-node>`"),
		expected: []RstRole{{Target: "mongodb-compatibility-table-about-node", RoleType: "ref", Name: "ref"}},
	}, {
		input:    []byte("- :v4.0:`https://docs.mongodb.com/v4.0 </tutorial/restore-sharded-cluster>`"),
		expected: []RstRole{{Target: "/tutorial/restore-sharded-cluster", RoleType: "role", Name: "v4.0"}},
	}, {
		input:    []byte(":doc:`Internal Authentication</core/security-internal-authentication>`."),
		expected: []RstRole{{Target: "/core/security-internal-authentication", RoleType: "role", Name: "doc"}},
	}, {
		input:    []byte(":authaction:`find`/:authaction:`update`"),
		expected: []RstRole{{Target: "find", RoleType: "role", Name: "authaction"}, {Target: "update", RoleType: "role", Name: "authaction"}},
	}}

	for _, test := range cases {
		got := ParseForRoles(test.input)
		assert.ElementsMatch(t, test.expected, got, "ParseForConstants(%q) should return %v, got %v", test.input, test.expected, got)
	}
}

func TestFindsSharedIncludes(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []SharedInclude
	}{{
		input:    []byte(""),
		expected: []SharedInclude{},
	}, {
		input:    []byte(".. code-block::"),
		expected: []SharedInclude{},
	}, {
		input:    []byte(".. important::"),
		expected: []SharedInclude{},
	}, {
		input:    []byte(".. include:: /includes/foo.txt"),
		expected: []SharedInclude{},
	}, {
		input:    []byte(".. sharedinclude:: dbx/about-compatibility.rst"),
		expected: []SharedInclude{{Path: "dbx/about-compatibility.rst"}},
	}}

	for _, test := range cases {
		got := ParseForSharedIncludes(test.input)
		assert.ElementsMatch(t, test.expected, got, "ParseForSharedIncludes(%q) should return %v, got %v", test.input, test.expected, got)
	}
}

func TestFindDirectives(t *testing.T) {
	cases := []struct {
		input    []byte
		expected []RstDirective
	}{{
		input:    []byte(""),
		expected: []RstDirective{},
	}, {
		input:    []byte(".. code-block::"),
		expected: []RstDirective{},
	}, {
		input:    []byte(".. important::"),
		expected: []RstDirective{},
	}, {
		input:    []byte(".. include:: /includes/foo.txt"),
		expected: []RstDirective{{Name: "include", Target: "/includes/foo.txt"}},
	}, {
		input:    []byte(".. sharedinclude:: dbx/about-compatibility.rst"),
		expected: []RstDirective{{Name: "sharedinclude", Target: "dbx/about-compatibility.rst"}},
	}, {
		input:    []byte(".. serverstatus:: metrics.repl.apply.batches.totalMillis"),
		expected: []RstDirective{{Name: "serverstatus", Target: "metrics.repl.apply.batches.totalMillis"}},
	}, {
		input:    []byte(".. method:: getMemInfo()"),
		expected: []RstDirective{{Name: "method", Target: "getMemInfo()"}},
	}, {
		input:    []byte(".. method:: sh.removeShardTag(shard, tag)"),
		expected: []RstDirective{{Name: "method", Target: "sh.removeShardTag(shard, tag)"}},
	}}

	for _, test := range cases {
		got := ParseForDirectives(test.input)
		assert.ElementsMatch(t, test.expected, got, "ParseForDirectives(%q) should return %v, got %v", test.input, test.expected, got)
	}
}
