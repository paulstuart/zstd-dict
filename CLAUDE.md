# Leveraging Zstd compression with dicts

This is an expirement to validate if using Zstd compression with pre-trained dicts might be an endeavor worth pursuing.
The current contents are from an initial expiriment to simply wrangle the functionality on the command line and can be ignored.

A key realization is that the value of using dicts is for smaller files where the size of the zstd dict can be larger than the resulting compressed data.

# Goals

* Create a Go package that supports using Zstd compression with pre-trained dicts form gRPC services.
* Create a demo project that sets up a simple client server pairing that uses a dataset that might be well-suited to this technique, e.g., a file listing of a directory
* Demonstrate the performance of this technique and compare it with a version that uses plain zstd, as well as gzip

# Anti-goals

* Do not try to create a comprehensive framework, just exercise and validate (or dispute) the assumptions driving this project

# Engineering guidance

* Use idiomatic Go and leverage language capabilities as of the current version (v1.25), e.g., testing/synctest)
* Use stdlib and avoid external dependencies unless otherwise warranted
* Avoid superfluous emojies and only use them where the situation truly calls for it
