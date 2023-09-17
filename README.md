# csdesync
A tool for detecting client side desync based on portswigger research:
https://portswigger.net/research/browser-powered-desync-attacks

# Installation
```
go install -v github.com/bassem-essam/csdesync@latest
```

# Usage
Usage with a single URL:
```
csdesync -u https://google.com
```

Usage with a list of URLs:
```
csdesync -f urls.txt
```

Help message:
```
Usage of csdesync
  -c int
        concurrency level (default 50)
  -f string
        a list of hosts to probe
  -o string
        output file (default "out.txt")
  -p string
        payload file (the body of the smuggle request)
  -u string
        a single url to probe
  -v    verbose
```

# Yep that's it
PRs or suggestions are undoubtedly welcome.
If you have any question or anything to say, please do not hesitate to ping me anytime.
