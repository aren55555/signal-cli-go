// Export dumps the account.db tables a capture replay needs to JSON, so the
// Go replay test can read signal-cli's SQLite database without an in-repo
// SQLite reader. It is a test tool run against a *copy* of a reference
// signal-cli data directory, never part of signal-cli-go.
//
// Run with the sqlite-jdbc driver bundled in the reference distribution:
//   java -cp third_party/signal-cli-0.14.7/lib/sqlite-jdbc-3.53.2.1.jar \
//     tools/capture-export/Export.java <copy of account.db> <output.json>
//
// Tables and columns follow spec/protocol/capture.md section 2. BLOBs are
// written as lowercase hex, integers as numbers, text as strings.

import java.nio.file.Files;
import java.nio.file.Path;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.sql.Types;
import java.util.HexFormat;

public class Export {
    static final String[] TABLES = {
        "pre_key", "signed_pre_key", "kyber_pre_key", "session", "identity", "sender_key",
    };

    public static void main(String[] args) throws Exception {
        if (args.length != 2) {
            System.err.println("usage: Export <account.db copy> <output.json>");
            System.exit(2);
        }
        StringBuilder out = new StringBuilder("{\n");
        try (Connection c = DriverManager.getConnection("jdbc:sqlite:file:" + args[0] + "?mode=ro")) {
            for (int t = 0; t < TABLES.length; t++) {
                out.append("  \"").append(TABLES[t]).append("\": [");
                try (ResultSet rs = c.createStatement().executeQuery("SELECT * FROM " + TABLES[t])) {
                    ResultSetMetaData md = rs.getMetaData();
                    boolean firstRow = true;
                    while (rs.next()) {
                        out.append(firstRow ? "\n    {" : ",\n    {");
                        firstRow = false;
                        for (int i = 1; i <= md.getColumnCount(); i++) {
                            if (i > 1) out.append(", ");
                            out.append('"').append(md.getColumnName(i)).append("\": ");
                            Object v = rs.getObject(i);
                            if (v == null) out.append("null");
                            else if (v instanceof byte[] b) out.append('"').append(HexFormat.of().formatHex(b)).append('"');
                            else if (v instanceof Number n) out.append(n);
                            else out.append('"').append(v.toString().replace("\\", "\\\\").replace("\"", "\\\"")).append('"');
                        }
                        out.append('}');
                    }
                }
                out.append(t == TABLES.length - 1 ? "\n  ]\n" : "\n  ],\n");
            }
        }
        out.append("}\n");
        Files.writeString(Path.of(args[1]), out);
    }
}
