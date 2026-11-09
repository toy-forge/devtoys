// Package tools 通过空导入把全部工具包串起来：
// 导入本包即触发各工具包 init() 中的 Register 自注册。
// 新增工具时，实现包并在此追加一行空导入即可。
package tools

import (
	_ "github.com/toy-forge/devtoys/internal/tools/converters/cronparser"
	_ "github.com/toy-forge/devtoys/internal/tools/converters/dateparser"
	_ "github.com/toy-forge/devtoys/internal/tools/converters/jsontable"
	_ "github.com/toy-forge/devtoys/internal/tools/converters/jsonyaml"
	_ "github.com/toy-forge/devtoys/internal/tools/converters/numberbase"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/base64"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/base64image"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/certificate"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/gzip"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/htmlenc"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/jwt"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/qrcode"
	_ "github.com/toy-forge/devtoys/internal/tools/encoders/urlenc"
	_ "github.com/toy-forge/devtoys/internal/tools/formatters/jsonfmt"
	_ "github.com/toy-forge/devtoys/internal/tools/formatters/sqlfmt"
	_ "github.com/toy-forge/devtoys/internal/tools/formatters/xmlfmt"
	_ "github.com/toy-forge/devtoys/internal/tools/generators/hash"
	_ "github.com/toy-forge/devtoys/internal/tools/generators/lorem"
	_ "github.com/toy-forge/devtoys/internal/tools/generators/password"
	_ "github.com/toy-forge/devtoys/internal/tools/generators/uuid"
	_ "github.com/toy-forge/devtoys/internal/tools/graphic/colorblind"
	_ "github.com/toy-forge/devtoys/internal/tools/graphic/imageconvert"
	_ "github.com/toy-forge/devtoys/internal/tools/testers/jsonpath"
	_ "github.com/toy-forge/devtoys/internal/tools/testers/regex"
	_ "github.com/toy-forge/devtoys/internal/tools/testers/xmlvalidator"
	_ "github.com/toy-forge/devtoys/internal/tools/testers/xpath"
	_ "github.com/toy-forge/devtoys/internal/tools/text/analyzer"
	_ "github.com/toy-forge/devtoys/internal/tools/text/comparer"
	_ "github.com/toy-forge/devtoys/internal/tools/text/escape"
	_ "github.com/toy-forge/devtoys/internal/tools/text/listcompare"
	_ "github.com/toy-forge/devtoys/internal/tools/text/markdown"
)
