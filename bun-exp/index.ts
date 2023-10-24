console.log("Hello via Bun!");


import tar from "tar";
import fs from "fs";
import zlib from "zlib";

fs.createReadStream("/home/ubuntu/.cache/Homebrew/downloads/843ec2129e032ac407cc17cf9141a6ce69f8f0556061f6e1de7ecee17f4ae971--ruby--3.2.2.x86_64_linux.bottle.tar.gz")
    .pipe(zlib.createGunzip())
    .pipe(tar.extract({ cwd: "./tmp"})).on("error", (e) => console.log(e))

